package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/go-basic/uuid"
	_ "github.com/lib/pq"
)

// 配置选项，支持命令行参数覆盖
var (
	// 数据库配置
	dbHost     = flag.String("db-host", "127.0.0.1:5432", "数据库服务器地址和端口")
	dbUser     = flag.String("db-user", "postgres", "数据库用户名")
	dbPassword = flag.String("db-pass", "ThingsPanel2023", "数据库密码")
	dbName     = flag.String("db-name", "thingspanel", "数据库名称")
	dbSSLMode  = flag.String("db-ssl", "disable", "数据库SSL模式")

	// 设备配置
	tenantID     = flag.String("tenant", "9c3f8a70", "租户ID")
	devicePrefix = flag.String("prefix", "2025.5.8测试", "设备名称前缀")
	deviceNumber = flag.String("number", "3", "设备名称后缀数字")
	deviceCount  = flag.Int("count", 3, "要创建的设备数量")
	batchSize    = flag.Int("batch", 100, "批量插入的大小")

	// 文件输出配置
	outputDir     = flag.String("output", ".", "输出文件目录")
	idFileName    = flag.String("id-file", "device_id.txt", "设备ID文件名")
	tokenFileName = flag.String("token-file", "device_username.txt", "设备Token文件名")
	appendMode    = flag.Bool("append", true, "是否追加写入文件")
)

// DeviceVoucher 设备凭证结构
type DeviceVoucher struct {
	Username string `json:"username"`
}

// Device 结构体表示要创建的设备
type Device struct {
	ID           string
	Name         string
	Token        string
	VoucherJSON  string
	CreationTime time.Time
}

func init() {
	// 解析命令行参数
	flag.Parse()

	// 设置日志格式
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
}

func main() {
	log.Println("开始创建测试设备...")

	// 连接数据库
	db, err := connectDB()
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer db.Close()

	// 创建输出目录
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		log.Fatalf("创建输出目录失败: %v", err)
	}

	// 生成设备并插入数据库
	devices, err := createDevices(db, *deviceCount)
	if err != nil {
		log.Fatalf("创建设备失败: %v", err)
	}
	log.Printf("成功创建 %d 个设备", len(devices))

	// 保存设备ID和Token到文件
	if err := saveDeviceInfo(devices); err != nil {
		log.Fatalf("保存设备信息到文件失败: %v", err)
	}

	log.Println("设备创建完成")
}

// connectDB 连接PostgreSQL数据库
func connectDB() (*sql.DB, error) {
	// 构建连接字符串
	connStr := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=%s",
		*dbUser, *dbPassword, *dbHost, *dbName, *dbSSLMode)

	// 连接数据库
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("无法连接数据库: %w", err)
	}

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("数据库连接测试失败: %w", err)
	}

	// 设置连接池参数
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	return db, nil
}

// createDevices 生成指定数量的设备并插入数据库
func createDevices(db *sql.DB, count int) ([]Device, error) {
	devices := make([]Device, 0, count)

	// 开始事务
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("开始事务失败: %w", err)
	}
	defer tx.Rollback() // 如果提交成功，这个回滚不会执行

	// 准备SQL语句
	stmt, err := tx.Prepare(`INSERT INTO devices (
		id, "name", voucher, tenant_id, is_enabled, activate_flag, 
		created_at, update_at, device_number, product_id, parent_id, 
		protocol, "label", "location", sub_device_addr, current_version, 
		additional_info, protocol_config, remark1, remark2, remark3, 
		device_config_id, batch_number, activate_at, is_online, access_way, 
		description, service_access_id) 
	VALUES (
		$1, $2, $3, $4, '', 'active', $5, $6, $7, 
		NULL, NULL, NULL, '', NULL, NULL, NULL, 
		'{}'::json, '{}'::json, NULL, NULL, NULL, 
		NULL, NULL, NULL, 0, 'A', NULL, NULL)`)
	if err != nil {
		return nil, fmt.Errorf("准备SQL语句失败: %w", err)
	}
	defer stmt.Close()

	// 批量创建设备
	log.Printf("开始创建 %d 个设备...", count)
	startTime := time.Now()

	// 检查是批量处理还是一次性处理
	batchCount := *batchSize
	if batchCount <= 0 || batchCount > count {
		batchCount = count
	}

	for i := 0; i < count; i++ {
		// 创建设备信息
		device := generateDevice(i)
		devices = append(devices, device)

		// 执行插入
		_, err = stmt.Exec(
			device.ID,
			device.Name,
			device.VoucherJSON,
			*tenantID,
			device.CreationTime,
			device.CreationTime,
			device.ID,
		)
		if err != nil {
			return nil, fmt.Errorf("插入设备数据失败(序号 %d): %w", i, err)
		}

		// 每批次提交一次事务
		if (i+1)%batchCount == 0 || i == count-1 {
			if err := tx.Commit(); err != nil {
				return nil, fmt.Errorf("提交事务失败: %w", err)
			}

			// 进度报告
			progress := float64(i+1) / float64(count) * 100
			log.Printf("进度: %.1f%% (%d/%d)", progress, i+1, count)

			// 如果还有更多设备要创建，开始新事务
			if i < count-1 {
				tx, err = db.Begin()
				if err != nil {
					return nil, fmt.Errorf("开始新事务失败: %w", err)
				}
				defer tx.Rollback()

				stmt, err = tx.Prepare(`INSERT INTO devices (
					id, "name", voucher, tenant_id, is_enabled, activate_flag, 
					created_at, update_at, device_number, product_id, parent_id, 
					protocol, "label", "location", sub_device_addr, current_version, 
					additional_info, protocol_config, remark1, remark2, remark3, 
					device_config_id, batch_number, activate_at, is_online, access_way, 
					description, service_access_id) 
				VALUES (
					$1, $2, $3, $4, '', 'active', $5, $6, $7, 
					NULL, NULL, NULL, '', NULL, NULL, NULL, 
					'{}'::json, '{}'::json, NULL, NULL, NULL, 
					NULL, NULL, NULL, 0, 'A', NULL, NULL)`)
				if err != nil {
					return nil, fmt.Errorf("准备新SQL语句失败: %w", err)
				}
				defer stmt.Close()
			}
		}
	}

	elapsed := time.Since(startTime)
	log.Printf("创建完成，耗时: %v，平均: %.2f 设备/秒",
		elapsed, float64(count)/elapsed.Seconds())

	return devices, nil
}

// generateDevice 生成单个设备信息
func generateDevice(index int) Device {
	id := uuid.New()
	token := uuid.New()
	now := time.Now()

	// 创建设备凭证
	voucher := DeviceVoucher{Username: token}
	voucherJSON, err := json.Marshal(voucher)
	if err != nil {
		log.Printf("警告: 序列化设备凭证失败: %v", err)
		// 使用空JSON对象作为后备方案
		voucherJSON = []byte("{}")
	}

	// 创建设备名称
	name := fmt.Sprintf("%s_%s_%d", *devicePrefix, *deviceNumber, index)

	return Device{
		ID:           id,
		Name:         name,
		Token:        token,
		VoucherJSON:  string(voucherJSON),
		CreationTime: now,
	}
}

// saveDeviceInfo 保存设备ID和Token到文件
func saveDeviceInfo(devices []Device) error {
	var idList []string
	var tokenList []string

	// 提取ID和Token
	for _, device := range devices {
		idList = append(idList, device.ID)
		tokenList = append(tokenList, device.Token)
	}

	// 创建文件写入函数
	writeFunc := WriteFile
	if *appendMode {
		writeFunc = AppendFile
	}

	// 保存ID
	idFilePath := filepath.Join(*outputDir, *idFileName)
	if err := writeFunc(idFilePath, idList); err != nil {
		return fmt.Errorf("写入ID文件失败: %w", err)
	}
	log.Printf("设备ID已保存到: %s", idFilePath)

	// 保存Token
	tokenFilePath := filepath.Join(*outputDir, *tokenFileName)
	if err := writeFunc(tokenFilePath, tokenList); err != nil {
		return fmt.Errorf("写入Token文件失败: %w", err)
	}
	log.Printf("设备Token已保存到: %s", tokenFilePath)

	return nil
}

// WriteFile 将字符串列表写入文件（覆盖模式）
func WriteFile(filepath string, lines []string) error {
	f, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("创建文件失败: %w", err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, line := range lines {
		fmt.Fprintln(w, line)
	}

	return w.Flush()
}

// AppendFile 将字符串列表追加到文件末尾
func AppendFile(filepath string, lines []string) error {
	f, err := os.OpenFile(filepath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, line := range lines {
		fmt.Fprintln(w, line)
	}

	return w.Flush()
}

// ReadFile 读取文件的每一行
func ReadFile(filepath string) ([]string, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" { // 忽略空行
			lines = append(lines, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取文件内容失败: %w", err)
	}

	return lines, nil
}
