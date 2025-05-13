package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// SensorData 表示设备上报的传感器数据结构（使用动态map）
type SensorData map[string]float64

// 全局计数变量
var (
	successNum uint64        // 成功连接的设备数
	dataCount  uint64        // 已发送的数据点数
	msgCount   uint64        // 已发送的消息数
	exitCount  uint64        // 已退出的goroutine数
	startChan  chan struct{} // 同步开始信号

	// 添加第一次发送数据的时间记录
	firstSendTime atomic.Value // 记录第一次发送数据的时间点
)

// 命令行参数定义(保留以支持命令行配置)
var (
	// 设备相关配置
	deviceTokenFile = flag.String("token-file", "", "设备token文件路径")
	clientNumber    = flag.Int("clients", 0, "模拟连接的设备数量")

	// MQTT相关配置
	mqttServer = flag.String("mqtt-server", "", "MQTT服务器地址")
	qos        = flag.Int("qos", -1, "MQTT服务质量(0,1,2)")
	topic      = flag.String("topic", "", "发布主题")

	// 测试参数配置
	dataInterval    = flag.Duration("interval", 0, "数据上报间隔时间")
	testCycleCount  = flag.Int("cycles", 0, "测试循环次数")
	connectWaitTime = flag.Duration("connect-wait", 0, "连接等待时间")

	// 数据参数
	minValue = flag.Float64("min-value", 0, "传感器数据最小值")
	maxValue = flag.Float64("max-value", 0, "传感器数据最大值")
)

func init() {
	// 初始化随机数生成器
	gofakeit.Seed(time.Now().UnixNano())

	// 设置MQTT日志
	mqtt.ERROR = log.New(os.Stderr, "[MQTT ERROR] ", log.LstdFlags)
}

func main() {
	// 加载配置
	LoadConfig()

	log.Println("性能测试开始")
	log.Printf("配置信息: 设备数=%d, 间隔时间=%v, 循环次数=%d",
		AppConfig.Device.ClientNumber,
		AppConfig.Test.DataInterval,
		AppConfig.Test.CycleCount)

	// 从文件中读取设备token
	tokenLines, err := readFile(AppConfig.Device.TokenFile)
	if err != nil {
		log.Fatalf("读取设备token文件失败: %v", err)
	}

	// 初始化通道
	startChan = make(chan struct{})

	// 初始化firstSendTime为nil表示尚未发送数据
	firstSendTime.Store((*time.Time)(nil))

	// 创建上下文，用于控制所有设备goroutine的生命周期
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // 确保在main函数退出时取消所有goroutine

	// 启动监控日志，并等待其初始化完成
	monitorInitDone := make(chan struct{})
	go func() {
		// 这里启动监控模块，并在监控初始化完成后发送信号
		MonitorLogs(monitorInitDone, &firstSendTime)
	}()

	// 等待监控初始化完成或超时
	select {
	case <-monitorInitDone:
		log.Println("监控模块初始化完成，开始进行测试...")
	case <-time.After(10 * time.Second):
		log.Println("警告: 监控模块初始化超时，继续进行测试...")
	}

	// 创建等待组，用于等待所有设备goroutine完成
	var wg sync.WaitGroup
	log.Printf("可用设备数量: %d", len(tokenLines))

	// 启动设备连接，每个设备一个goroutine
	availableDevices := len(tokenLines)
	if availableDevices < AppConfig.Device.ClientNumber {
		log.Printf("警告: 可用设备数量(%d)少于请求数量(%d)", availableDevices, AppConfig.Device.ClientNumber)
		AppConfig.Device.ClientNumber = availableDevices
	}

	for i := 0; i < AppConfig.Device.ClientNumber; i++ {
		wg.Add(1)
		go connectAndPublish(&wg, ctx, tokenLines[i])
	}

	// 等待设备连接完成
	time.Sleep(AppConfig.Test.ConnectWaitTime)

	connectedDevices := atomic.LoadUint64(&successNum)
	log.Printf("成功连接设备数: %d (%.1f%%)", connectedDevices, float64(connectedDevices)*100/float64(AppConfig.Device.ClientNumber))

	if connectedDevices == 0 {
		log.Println("没有设备连接成功，测试终止")
		cancel()
		wg.Wait()
		return
	}

	// 创建测试开始时间变量，但实际值在第一次发送时设置
	testStartTime := time.Now()
	nextSendTime := time.Now()

	// 主测试循环
	for cycle := 1; cycle <= AppConfig.Test.CycleCount; cycle++ {
		// 计算此次发送的目标时间
		nextSendTime = nextSendTime.Add(AppConfig.Test.DataInterval)

		// 计算需要等待的时间
		waitTime := time.Until(nextSendTime)
		if waitTime > 0 {
			time.Sleep(waitTime)
		}

		// 触发所有设备同时发送数据
		close(startChan)

		// 如果是第一次发送数据，记录时间
		if cycle == 1 {
			now := time.Now()
			firstSendTime.Store(&now)
			testStartTime = now // 同步更新testStartTime
		}

		// 创建新的触发通道，用于下一轮测试
		startChan = make(chan struct{})

		if AppConfig.Monitor.LogCycle {
			currentDataCount := atomic.LoadUint64(&dataCount)
			currentMsgCount := atomic.LoadUint64(&msgCount)

			// 从第一次发送开始计算速率
			pointsPerSecond := float64(currentDataCount) / time.Since(testStartTime).Seconds()
			msgsPerSecond := float64(currentMsgCount) / time.Since(testStartTime).Seconds()

			log.Printf("循环 %d/%d: 已发送数据点数: %d (%.1f点/秒), 消息数: %d (%.1f消息/秒)",
				cycle, AppConfig.Test.CycleCount, currentDataCount, pointsPerSecond,
				currentMsgCount, msgsPerSecond)
		}
	}

	// 测试完成，关闭所有设备连接
	cancel()
	testDuration := time.Since(testStartTime)

	// 输出测试结果
	log.Printf("等待所有设备退出...")
	wg.Wait()

	// 获取最终统计
	finalDataCount := atomic.LoadUint64(&dataCount)
	finalMsgCount := atomic.LoadUint64(&msgCount)
	finalExitCount := atomic.LoadUint64(&exitCount)

	// 打印简要测试总结
	log.Println("\n========== 测试完成 ==========")
	log.Printf("测试总耗时: %v", testDuration)
	log.Printf("测试循环次数: %d", AppConfig.Test.CycleCount)
	log.Printf("已退出设备数: %d (%.1f%%)", finalExitCount, float64(finalExitCount)*100/float64(AppConfig.Device.ClientNumber))
	log.Printf("总发送数据点数: %d", finalDataCount)
	log.Printf("总发送消息数: %d", finalMsgCount)
	log.Println("===============================")
	log.Println("\n测试已完成。监控线程仍在运行，可以继续观察数据入库情况。")
	log.Println("按 Enter 键退出程序...")

	// 创建一个通道用于接收输入完成信号
	inputDone := make(chan struct{})

	// 启动一个goroutine等待用户输入
	go func() {
		// 读取一行输入(等待按Enter键)
		reader := bufio.NewReader(os.Stdin)
		_, _ = reader.ReadString('\n')
		close(inputDone)
	}()

	// 等待用户输入或者CTRL+C信号
	<-inputDone

	log.Println("程序正在退出...")
}

// readFile 从指定的文件中读取每一行内容并返回字符串切片
func readFile(fileName string) ([]string, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" { // 忽略空行
			lines = append(lines, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取文件内容失败: %w", err)
	}

	if len(lines) == 0 {
		return nil, fmt.Errorf("文件为空或不包含有效设备token")
	}

	return lines, nil
}

// connectAndPublish 连接MQTT服务器并定时发布传感器数据
func connectAndPublish(wg *sync.WaitGroup, ctx context.Context, username string) {
	defer wg.Done()
	defer func() {
		atomic.AddUint64(&exitCount, 1)
	}()

	// 设置MQTT客户端选项
	clientID := username + "_" + time.Now().Format("150405")
	opts := mqtt.NewClientOptions().
		SetClientID(clientID).
		AddBroker(AppConfig.MQTT.Server).
		SetUsername(username).
		SetCleanSession(true).
		SetAutoReconnect(true).
		SetKeepAlive(60 * time.Second).
		SetMaxReconnectInterval(5 * time.Second)

	// 创建并连接MQTT客户端
	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Printf("设备 %s 连接MQTT服务器失败: %v", username, token.Error())
		return
	}

	// 连接成功，计数器加1
	atomic.AddUint64(&successNum, 1)
	defer client.Disconnect(200) // 确保在函数结束时断开连接

	// 预生成传感器数据对象，避免频繁创建
	sensorData := make(SensorData)

	// 主循环：等待触发信号并发送数据
	for {
		select {
		case <-ctx.Done(): // 测试结束信号
			return
		default:
			<-startChan // 等待开始信号

			// 生成模拟传感器数据
			updateSensorData(sensorData)

			// 将数据序列化为JSON
			jsonData, err := json.Marshal(sensorData)
			if err != nil {
				log.Printf("序列化数据失败: %v", err)
				continue
			}

			// 发布数据到MQTT主题
			token := client.Publish(AppConfig.MQTT.Topic, byte(AppConfig.MQTT.QoS), false, jsonData)
			token.Wait()

			if token.Error() != nil {
				log.Printf("发布消息失败: %v", token.Error())
			} else {
				// 每条消息包含配置的数据点数量
				atomic.AddUint64(&dataCount, uint64(len(sensorData)))
				atomic.AddUint64(&msgCount, 1)
			}

			// 让出CPU时间片，避免单个goroutine占用过多资源
			runtime.Gosched()
		}
	}
}

// updateSensorData 更新传感器数据对象的值
func updateSensorData(data SensorData) {
	// 清空旧数据
	for k := range data {
		delete(data, k)
	}

	// 根据配置生成指定数量的数据点
	for i := 1; i <= AppConfig.Data.DataPointCount; i++ {
		key := fmt.Sprintf("hum%d", i)
		data[key] = gofakeit.Float64Range(AppConfig.Data.MinValue, AppConfig.Data.MaxValue)
	}
}
