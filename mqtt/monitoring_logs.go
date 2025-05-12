package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	_ "github.com/lib/pq"
)

// MonitorLogs 监控数据库写入状态和对比已发送数据点数
func MonitorLogs(initDone chan<- struct{}) {
	// 连接数据库
	connStr := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=%s",
		AppConfig.Database.User,
		AppConfig.Database.Password,
		AppConfig.Database.Host,
		AppConfig.Database.Name,
		AppConfig.Database.SSLMode)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Printf("监控模块: 无法连接数据库: %v", err)
		close(initDone) // 通知初始化完成(虽然失败)
		return
	}
	defer db.Close()

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Printf("监控模块: 数据库连接测试失败: %v", err)
		close(initDone) // 通知初始化完成(虽然失败)
		return
	}

	log.Printf("监控模块: 成功连接到数据库，开始监控数据写入情况，监控间隔: %v", AppConfig.Monitor.LogInterval)

	// 查询初始值作为基准
	var initialCount int64
	err = db.QueryRow("SELECT COUNT(*) FROM telemetry_datas").Scan(&initialCount)
	if err != nil {
		log.Printf("监控模块: 获取初始数据点数失败: %v", err)
		initialCount = 0
	}

	log.Printf("监控模块: 数据库中当前数据点数: %d", initialCount)

	// 初始发送点数
	initialSentCount := atomic.LoadUint64(&dataCount)
	initialMsgCount := atomic.LoadUint64(&msgCount)
	lastDBCount := initialCount
	lastSentCount := initialSentCount
	lastMsgCount := initialMsgCount

	// 输出初始监控信息
	log.Printf("\n========== 初始监控状态 ==========")
	log.Printf("数据库初始数据点数: %d", initialCount)
	log.Printf("==============================")

	// 通知初始化完成，测试可以开始
	close(initDone)

	// 定时监控循环
	ticker := time.NewTicker(AppConfig.Monitor.LogInterval)
	defer ticker.Stop()

	startTime := time.Now()

	for {
		<-ticker.C

		// 当前已发送点数
		currentSentCount := atomic.LoadUint64(&dataCount)
		currentMsgCount := atomic.LoadUint64(&msgCount)
		sentDiff := currentSentCount - lastSentCount
		msgDiff := currentMsgCount - lastMsgCount

		// 查询当前数据库点数
		var currentDBCount int64
		err = db.QueryRow("SELECT COUNT(*) FROM telemetry_datas").Scan(&currentDBCount)
		if err != nil {
			log.Printf("监控模块: 查询数据库点数失败: %v", err)
			continue
		}

		dbDiff := currentDBCount - lastDBCount
		elapsedTime := time.Since(startTime)

		// 计算统计信息
		sentRate := float64(sentDiff) / AppConfig.Monitor.LogInterval.Seconds()
		msgRate := float64(msgDiff) / AppConfig.Monitor.LogInterval.Seconds()
		dbRate := float64(dbDiff) / AppConfig.Monitor.LogInterval.Seconds()
		totalSentRate := float64(currentSentCount) / elapsedTime.Seconds()
		totalMsgRate := float64(currentMsgCount) / elapsedTime.Seconds()
		totalDBRate := float64(currentDBCount-initialCount) / elapsedTime.Seconds()

		// 计算写入成功率
		successRate := 0.0
		if sentDiff > 0 {
			successRate = float64(dbDiff) / float64(sentDiff) * 100.0
			// 限制最大显示为100%
			if successRate > 100.0 {
				successRate = 100.0
			}
		}

		// 累计成功率
		totalSuccessRate := 0.0
		if currentSentCount > 0 {
			totalSuccessRate = float64(currentDBCount-initialCount) / float64(currentSentCount) * 100.0
			// 限制最大显示为100%
			if totalSuccessRate > 100.0 {
				totalSuccessRate = 100.0
			}
		}

		// 打印监控信息
		log.Printf("\n========== 监控报告 ==========")
		log.Printf("已运行时间: %v", elapsedTime.Round(time.Second))
		log.Printf("当前配置: 每条消息数据点数: %d", AppConfig.Data.DataPointCount)
		log.Printf("当前间隔(%v)统计:", AppConfig.Monitor.LogInterval)
		log.Printf("  - 已发送数据点: %d (本次新增: %d), 速率: %.1f 点/秒",
			currentSentCount, sentDiff, sentRate)
		log.Printf("  - 已发送消息: %d (本次新增: %d), 速率: %.1f 条/秒",
			currentMsgCount, msgDiff, msgRate)
		log.Printf("  - 数据库记录数: %d (本次新增: %d), 速率: %.1f 点/秒",
			currentDBCount, dbDiff, dbRate)

		// 只在有新数据时显示写入率
		if sentDiff > 0 {
			log.Printf("  - 本次写入率: %.1f%% (数据库新增/发送新增)", successRate)
		}

		log.Printf("累计统计:")
		log.Printf("  - 总发送数据点: %d, 平均速率: %.1f 点/秒",
			currentSentCount, totalSentRate)
		log.Printf("  - 总发送消息: %d, 平均速率: %.1f 条/秒",
			currentMsgCount, totalMsgRate)
		log.Printf("  - 总入库数据点: %d, 平均速率: %.1f 点/秒",
			currentDBCount-initialCount, totalDBRate)

		// 有数据发送时才计算成功率和平均值
		if currentSentCount > 0 {
			log.Printf("  - 总体写入率: %.1f%% (总入库/总发送)", totalSuccessRate)

			// 如果数据库新增明显超过发送量，给出提示
			if totalSuccessRate > 95.0 {
				log.Printf("  - 注意: 数据库可能还在处理之前的数据")
			}
		}

		// 显示消息相关统计
		if currentMsgCount > 0 {
			// 计算实际平均每条消息的数据点数
			avgPointsPerMsg := float64(currentSentCount) / float64(currentMsgCount)
			log.Printf("  - 实际平均每条消息数据点数: %.2f", avgPointsPerMsg)
		}

		// 如果配置了数据点数，计算基于数据点的理论消息数(用于与实际消息数对比验证)
		if AppConfig.Data.DataPointCount > 0 && currentSentCount > 0 {
			theoreticalMsgCount := currentSentCount / uint64(AppConfig.Data.DataPointCount)
			log.Printf("  - 基于数据点计算的理论消息数: %d (用于验证)", theoreticalMsgCount)

			// 如果有实际消息，计算理论值与实际值的差异率
			if currentMsgCount > 0 {
				diffRate := (float64(theoreticalMsgCount) - float64(currentMsgCount)) / float64(currentMsgCount) * 100.0
				log.Printf("  - 理论值与实际值差异: %.2f%%", diffRate)
			}
		}

		log.Printf("==============================")

		// 更新上次统计值
		lastDBCount = currentDBCount
		lastSentCount = currentSentCount
		lastMsgCount = currentMsgCount
	}
}
