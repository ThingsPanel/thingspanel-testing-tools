# ThingsPanel性能测试工具

这是一个用于测试ThingsPanel物联网平台性能的工具集，主要包含设备创建和MQTT性能测试两个主要功能模块。

## 功能特点

### 1. 设备创建模块
- 支持批量创建设备
- 自动生成设备ID和Token
- 支持将设备信息保存到文件
- 支持追加模式写入
- 支持自定义设备名称前缀和后缀

### 2. MQTT性能测试模块
- 支持多设备并发连接
- 支持自定义数据上报间隔
- 支持自定义测试循环次数
- 支持自定义数据点数量
- 实时监控数据写入情况
- 支持数据库写入性能监控
- 提供详细的测试报告

## 使用方法

### 1. 创建设备

```bash
cd create_device
go run main.go [参数]
```

主要参数说明：
- `--db-host`: 数据库服务器地址和端口（默认：127.0.0.1:5432）
- `--db-user`: 数据库用户名（默认：postgres）
- `--db-pass`: 数据库密码（默认：ThingsPanel2023）
- `--db-name`: 数据库名称（默认：thingspanel）
- `--tenant`: 租户ID（默认：9c3f8a70）
- `--prefix`: 设备名称前缀（默认：2025.5.8测试）
- `--number`: 设备名称后缀数字（默认：3）
- `--count`: 要创建的设备数量（默认：3）
- `--batch`: 批量插入的大小（默认：100）
- `--output`: 输出文件目录（默认：当前目录）
- `--id-file`: 设备ID文件名（默认：device_id.txt）
- `--token-file`: 设备Token文件名（默认：device_username.txt）
- `--append`: 是否追加写入文件（默认：true）

### 2. MQTT性能测试

```bash
cd mqtt
go run main.go [参数]
```

主要参数说明：
- `--config`: 配置文件路径（默认：config.yml）
- `--token-file`: 设备token文件路径
- `--clients`: 模拟连接的设备数量
- `--mqtt-server`: MQTT服务器地址
- `--qos`: MQTT服务质量(0,1,2)
- `--topic`: 发布主题
- `--interval`: 数据上报间隔时间
- `--cycles`: 测试循环次数
- `--connect-wait`: 连接等待时间
- `--min-value`: 传感器数据最小值
- `--max-value`: 传感器数据最大值
- `--data-points`: 每条消息包含的数据点数量

## 配置文件说明

配置文件（config.yml）包含以下主要配置项：

```yaml
# 设备相关配置
device:
  token_file: "../create_device/device_username.txt"  # 设备token文件路径
  client_number: 5                                     # 模拟连接的设备数量

# MQTT相关配置
mqtt:
  server: "127.0.0.1:1883"  # MQTT服务器地址
  qos: 0                        # MQTT服务质量(0,1,2)
  topic: "devices/telemetry"    # 发布主题

# 测试参数配置
test:
  data_interval: 100ms          # 数据上报间隔时间
  cycle_count: 200              # 测试循环次数
  connect_wait_time: 3s         # 连接等待时间

# 数据参数
data:
  min_value: 1.0                # 传感器数据最小值
  max_value: 10.0               # 传感器数据最大值
  data_point_count: 10          # 每条消息包含的数据点数量

# 数据库配置
database:
  host: "127.0.0.1:5432"    # 数据库服务器地址和端口
  user: "postgres"              # 数据库用户名
  password: "ThingsPanel2023"   # 数据库密码
  name: "thingspanel"              # 数据库名称
  ssl_mode: "disable"           # 数据库SSL模式

# 监控配置
monitor:
  log_interval: 10s             # 日志输出间隔
```

## 测试报告

测试完成后，工具会生成详细的测试报告，包括：
- 测试总耗时
- 测试循环次数
- 成功连接设备数
- 总发送数据点数
- 平均吞吐量
- 每个数据点平均耗时
- 数据库写入性能统计

## 注意事项

1. 使用前请确保已正确配置数据库连接信息
2. 建议先使用少量设备进行测试，确认配置无误后再进行大规模测试
3. 注意监控服务器资源使用情况，避免资源耗尽
4. 建议在测试环境中进行测试，避免影响生产环境