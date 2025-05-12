package main

import (
	"flag"
	"log"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 应用程序配置
type Config struct {
	Device struct {
		TokenFile    string `yaml:"token_file"`    // 设备token文件路径
		ClientNumber int    `yaml:"client_number"` // 模拟连接的设备数量
	} `yaml:"device"`

	MQTT struct {
		Server string `yaml:"server"` // MQTT服务器地址
		QoS    int    `yaml:"qos"`    // MQTT服务质量(0,1,2)
		Topic  string `yaml:"topic"`  // 发布主题
	} `yaml:"mqtt"`

	Test struct {
		DataInterval    time.Duration `yaml:"data_interval"`     // 数据上报间隔时间
		CycleCount      int           `yaml:"cycle_count"`       // 测试循环次数
		ConnectWaitTime time.Duration `yaml:"connect_wait_time"` // 连接等待时间
	} `yaml:"test"`

	Data struct {
		MinValue       float64 `yaml:"min_value"`        // 传感器数据最小值
		MaxValue       float64 `yaml:"max_value"`        // 传感器数据最大值
		DataPointCount int     `yaml:"data_point_count"` // 每条消息包含的数据点数量
	} `yaml:"data"`

	Database struct {
		Host     string `yaml:"host"`     // 数据库服务器地址和端口
		User     string `yaml:"user"`     // 数据库用户名
		Password string `yaml:"password"` // 数据库密码
		Name     string `yaml:"name"`     // 数据库名称
		SSLMode  string `yaml:"ssl_mode"` // 数据库SSL模式
	} `yaml:"database"`

	Monitor struct {
		LogInterval time.Duration `yaml:"log_interval"` // 日志输出间隔
	} `yaml:"monitor"`
}

// AppConfig 全局配置变量
var AppConfig Config

// 命令行参数定义
var configFile = flag.String("config", "config.yml", "配置文件路径")

// 数据库相关命令行参数
var (
	dbHost     = flag.String("db-host", "", "数据库服务器地址和端口")
	dbUser     = flag.String("db-user", "", "数据库用户名")
	dbPassword = flag.String("db-pass", "", "数据库密码")
	dbName     = flag.String("db-name", "", "数据库名称")
	dbSSLMode  = flag.String("db-ssl", "", "数据库SSL模式")
)

// 监控相关命令行参数
var logInterval = flag.Duration("log-interval", 0, "日志输出间隔")

// 数据点配置
var dataPointCount = flag.Int("data-points", 0, "每条消息包含的数据点数量")

// LoadConfig 加载配置
func LoadConfig() {
	// 解析命令行参数
	flag.Parse()

	// 读取配置文件
	configData, err := os.ReadFile(*configFile)
	if err != nil {
		log.Printf("读取配置文件 %s 失败: %v", *configFile, err)
		log.Println("使用默认配置和命令行参数")
	} else {
		// 解析YAML配置
		if err := yaml.Unmarshal(configData, &AppConfig); err != nil {
			log.Fatalf("解析配置文件失败: %v", err)
		}
	}

	// 命令行参数覆盖配置文件
	overrideConfigWithFlags()

	// 设置默认值（如果未指定）
	if AppConfig.Data.DataPointCount <= 0 {
		AppConfig.Data.DataPointCount = 10 // 默认10个数据点
		log.Printf("数据点数量未指定，使用默认值: %d", AppConfig.Data.DataPointCount)
	} else {
		log.Printf("使用配置的数据点数量: %d", AppConfig.Data.DataPointCount)
	}

	// 输出最终配置
	log.Println("当前配置:")
	log.Printf("- 设备配置: 文件=%s, 数量=%d",
		AppConfig.Device.TokenFile, AppConfig.Device.ClientNumber)
	log.Printf("- MQTT配置: 服务器=%s, QoS=%d, 主题=%s",
		AppConfig.MQTT.Server, AppConfig.MQTT.QoS, AppConfig.MQTT.Topic)
	log.Printf("- 测试配置: 间隔=%v, 循环=%d, 等待=%v",
		AppConfig.Test.DataInterval, AppConfig.Test.CycleCount, AppConfig.Test.ConnectWaitTime)
	log.Printf("- 数据配置: 最小值=%.1f, 最大值=%.1f, 数据点数=%d",
		AppConfig.Data.MinValue, AppConfig.Data.MaxValue, AppConfig.Data.DataPointCount)
	log.Printf("- 数据库配置: 主机=%s, 用户=%s, 数据库=%s",
		AppConfig.Database.Host, AppConfig.Database.User, AppConfig.Database.Name)
	log.Printf("- 监控配置: 日志间隔=%v",
		AppConfig.Monitor.LogInterval)
}

// overrideConfigWithFlags 使用命令行参数覆盖配置文件
func overrideConfigWithFlags() {
	// 设备配置
	if *deviceTokenFile != "" {
		AppConfig.Device.TokenFile = *deviceTokenFile
	}
	if *clientNumber > 0 {
		AppConfig.Device.ClientNumber = *clientNumber
	}

	// MQTT配置
	if *mqttServer != "" {
		AppConfig.MQTT.Server = *mqttServer
	}
	if *qos >= 0 {
		AppConfig.MQTT.QoS = *qos
	}
	if *topic != "" {
		AppConfig.MQTT.Topic = *topic
	}

	// 测试配置
	if *dataInterval > 0 {
		AppConfig.Test.DataInterval = *dataInterval
	}
	if *testCycleCount > 0 {
		AppConfig.Test.CycleCount = *testCycleCount
	}
	if *connectWaitTime > 0 {
		AppConfig.Test.ConnectWaitTime = *connectWaitTime
	}

	// 数据配置
	if *minValue > 0 {
		AppConfig.Data.MinValue = *minValue
	}
	if *maxValue > 0 {
		AppConfig.Data.MaxValue = *maxValue
	}
	if *dataPointCount > 0 {
		AppConfig.Data.DataPointCount = *dataPointCount
	}

	// 数据库配置
	if *dbHost != "" {
		AppConfig.Database.Host = *dbHost
	}
	if *dbUser != "" {
		AppConfig.Database.User = *dbUser
	}
	if *dbPassword != "" {
		AppConfig.Database.Password = *dbPassword
	}
	if *dbName != "" {
		AppConfig.Database.Name = *dbName
	}
	if *dbSSLMode != "" {
		AppConfig.Database.SSLMode = *dbSSLMode
	}

	// 监控配置
	if *logInterval > 0 {
		AppConfig.Monitor.LogInterval = *logInterval
	}
}
