# Mobile 流量监控集成示例

这个示例展示如何在移动应用中集成 Outline SDK 的流量监控功能。

## iOS 集成示例

### 1. Swift 代码示例

```swift
import UIKit
import Mobileproxy

class ProxyViewController: UIViewController {
    private var proxy: MobileproxyProxy?
    private var timer: Timer?
    
    @IBOutlet weak var uploadLabel: UILabel!
    @IBOutlet weak var downloadLabel: UILabel!
    @IBOutlet weak var connectionTimeLabel: UILabel!
    @IBOutlet weak var sessionTimeLabel: UILabel!
    @IBOutlet weak var activeConnectionsLabel: UILabel!
    @IBOutlet weak var totalConnectionsLabel: UILabel!
    
    override func viewDidLoad() {
        super.viewDidLoad()
        setupProxy()
        startMonitoring()
    }
    
    private func setupProxy() {
        do {
            // 创建 StreamDialer
            let transportConfig = "ss://your-shadowsocks-config"
            let dialer = try MobileproxyNewStreamDialerFromConfig(transportConfig)
            
            // 启动代理
            self.proxy = try MobileproxyRunProxy("localhost:0", dialer)
            
            print("代理启动成功，地址: \(proxy?.address() ?? "unknown")")
            
            // 配置应用的网络库使用这个代理
            configureNetworking()
            
        } catch {
            print("代理启动失败: \(error)")
        }
    }
    
    private func configureNetworking() {
        guard let proxy = proxy else { return }
        
        // 配置 URLSession 使用代理
        let proxyConfig = [
            "HTTPEnable": 1,
            "HTTPProxy": proxy.host(),
            "HTTPPort": proxy.port(),
            "HTTPSEnable": 1,
            "HTTPSProxy": proxy.host(),
            "HTTPSPort": proxy.port()
        ] as [String : Any]
        
        let configuration = URLSessionConfiguration.default
        configuration.connectionProxyDictionary = proxyConfig
        
        // 创建使用代理的 URLSession
        let session = URLSession(configuration: configuration)
        
        // 示例请求
        let url = URL(string: "https://httpbin.org/get")!
        session.dataTask(with: url) { data, response, error in
            if let error = error {
                print("请求失败: \(error)")
            } else {
                print("请求成功")
            }
        }.resume()
    }
    
    private func startMonitoring() {
        timer = Timer.scheduledTimer(withTimeInterval: 1.0, repeats: true) { _ in
            self.updateStatistics()
        }
    }
    
    private func updateStatistics() {
        guard let proxy = proxy else { return }
        
        DispatchQueue.main.async {
            // 更新流量统计
            let uploadBytes = proxy.getUploadBytes()
            let downloadBytes = proxy.getDownloadBytes()
            let connectionTime = proxy.getTotalConnectionTime()
            let sessionTime = proxy.getCurrentSessionDuration()
            let activeConnections = proxy.getActiveConnections()
            let totalConnections = proxy.getTotalConnections()
            
            // 格式化显示
            self.uploadLabel.text = "上传: \(self.formatBytes(uploadBytes))"
            self.downloadLabel.text = "下载: \(self.formatBytes(downloadBytes))"
            self.connectionTimeLabel.text = "连接时长: \(self.formatDuration(connectionTime))"
            self.sessionTimeLabel.text = "会话时长: \(self.formatDuration(sessionTime))"
            self.activeConnectionsLabel.text = "活跃连接: \(activeConnections)"
            self.totalConnectionsLabel.text = "总连接数: \(totalConnections)"
        }
    }
    
    private func formatBytes(_ bytes: Int64) -> String {
        let formatter = ByteCountFormatter()
        formatter.countStyle = .binary
        return formatter.string(fromByteCount: bytes)
    }
    
    private func formatDuration(_ milliseconds: Int64) -> String {
        let seconds = Double(milliseconds) / 1000.0
        return String(format: "%.1f秒", seconds)
    }
    
    @IBAction func resetStatsButtonTapped(_ sender: UIButton) {
        proxy?.resetTrafficStats()
        updateStatistics()
    }
    
    deinit {
        timer?.invalidate()
        proxy?.stop(5) // 5秒超时
    }
}
```

### 2. Objective-C 代码示例

```objc
#import "ProxyViewController.h"
#import <Mobileproxy/Mobileproxy.h>

@interface ProxyViewController ()
@property (nonatomic, strong) MobileproxyProxy *proxy;
@property (nonatomic, strong) NSTimer *monitoringTimer;

@property (weak, nonatomic) IBOutlet UILabel *uploadLabel;
@property (weak, nonatomic) IBOutlet UILabel *downloadLabel;
@property (weak, nonatomic) IBOutlet UILabel *connectionTimeLabel;
@property (weak, nonatomic) IBOutlet UILabel *sessionTimeLabel;
@end

@implementation ProxyViewController

- (void)viewDidLoad {
    [super viewDidLoad];
    [self setupProxy];
    [self startMonitoring];
}

- (void)setupProxy {
    NSError *error = nil;
    
    // 创建 StreamDialer
    NSString *transportConfig = @"ss://your-shadowsocks-config";
    MobileproxyStreamDialer *dialer = MobileproxyNewStreamDialerFromConfig(transportConfig, &error);
    
    if (error) {
        NSLog(@"创建 dialer 失败: %@", error);
        return;
    }
    
    // 启动代理
    self.proxy = MobileproxyRunProxy(@"localhost:0", dialer, &error);
    
    if (error) {
        NSLog(@"启动代理失败: %@", error);
        return;
    }
    
    NSLog(@"代理启动成功，地址: %@", [self.proxy address]);
}

- (void)startMonitoring {
    self.monitoringTimer = [NSTimer scheduledTimerWithTimeInterval:1.0
                                                            target:self
                                                          selector:@selector(updateStatistics)
                                                          userInfo:nil
                                                           repeats:YES];
}

- (void)updateStatistics {
    if (!self.proxy) return;
    
    dispatch_async(dispatch_get_main_queue(), ^{
        // 获取统计数据
        long long uploadBytes = [self.proxy getUploadBytes];
        long long downloadBytes = [self.proxy getDownloadBytes];
        long long connectionTime = [self.proxy getTotalConnectionTime];
        long long sessionTime = [self.proxy getCurrentSessionDuration];
        
        // 更新 UI
        self.uploadLabel.text = [NSString stringWithFormat:@"上传: %@", [self formatBytes:uploadBytes]];
        self.downloadLabel.text = [NSString stringWithFormat:@"下载: %@", [self formatBytes:downloadBytes]];
        self.connectionTimeLabel.text = [NSString stringWithFormat:@"连接时长: %.1f秒", connectionTime / 1000.0];
        self.sessionTimeLabel.text = [NSString stringWithFormat:@"会话时长: %.1f秒", sessionTime / 1000.0];
    });
}

- (NSString *)formatBytes:(long long)bytes {
    NSByteCountFormatter *formatter = [[NSByteCountFormatter alloc] init];
    formatter.countStyle = NSByteCountFormatterCountStyleBinary;
    return [formatter stringFromByteCount:bytes];
}

- (IBAction)resetStatsButtonTapped:(UIButton *)sender {
    [self.proxy resetTrafficStats];
    [self updateStatistics];
}

- (void)dealloc {
    [self.monitoringTimer invalidate];
    [self.proxy stop:5]; // 5秒超时
}

@end
```

## Android 集成示例

### Kotlin 代码示例

```kotlin
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import androidx.appcompat.app.AppCompatActivity
import kotlinx.android.synthetic.main.activity_proxy.*
import mobileproxy.Mobileproxy
import mobileproxy.Proxy
import mobileproxy.StreamDialer
import java.text.DecimalFormat

class ProxyActivity : AppCompatActivity() {
    private var proxy: Proxy? = null
    private val handler = Handler(Looper.getMainLooper())
    private var monitoringRunnable: Runnable? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_proxy)
        
        setupProxy()
        startMonitoring()
        
        resetButton.setOnClickListener {
            proxy?.resetTrafficStats()
            updateStatistics()
        }
    }

    private fun setupProxy() {
        try {
            // 创建 StreamDialer
            val transportConfig = "ss://your-shadowsocks-config"
            val dialer = Mobileproxy.newStreamDialerFromConfig(transportConfig)
            
            // 启动代理
            proxy = Mobileproxy.runProxy("localhost:0", dialer)
            
            println("代理启动成功，地址: ${proxy?.address()}")
            
            // 配置 OkHttp 使用代理
            configureNetworking()
            
        } catch (e: Exception) {
            println("代理启动失败: $e")
        }
    }

    private fun configureNetworking() {
        proxy?.let { proxy ->
            // 配置 OkHttp 客户端使用代理
            val proxyConfig = java.net.Proxy(
                java.net.Proxy.Type.HTTP,
                java.net.InetSocketAddress(proxy.host(), proxy.port().toInt())
            )
            
            val client = okhttp3.OkHttpClient.Builder()
                .proxy(proxyConfig)
                .build()
            
            // 使用配置好的客户端发送请求
            // ...
        }
    }

    private fun startMonitoring() {
        monitoringRunnable = object : Runnable {
            override fun run() {
                updateStatistics()
                handler.postDelayed(this, 1000) // 每秒更新一次
            }
        }
        handler.post(monitoringRunnable!!)
    }

    private fun updateStatistics() {
        proxy?.let { proxy ->
            runOnUiThread {
                // 获取统计数据
                val uploadBytes = proxy.getUploadBytes()
                val downloadBytes = proxy.getDownloadBytes()
                val connectionTime = proxy.getTotalConnectionTime()
                val sessionTime = proxy.getCurrentSessionDuration()
                val activeConnections = proxy.getActiveConnections()
                val totalConnections = proxy.getTotalConnections()

                // 更新 UI
                uploadText.text = "上传: ${formatBytes(uploadBytes)}"
                downloadText.text = "下载: ${formatBytes(downloadBytes)}"
                connectionTimeText.text = "连接时长: ${formatDuration(connectionTime)}"
                sessionTimeText.text = "会话时长: ${formatDuration(sessionTime)}"
                activeConnectionsText.text = "活跃连接: $activeConnections"
                totalConnectionsText.text = "总连接数: $totalConnections"
            }
        }
    }

    private fun formatBytes(bytes: Long): String {
        if (bytes < 1024) return "$bytes B"
        val exp = (Math.log(bytes.toDouble()) / Math.log(1024.0)).toInt()
        val pre = "KMGTPE"[exp - 1]
        val format = DecimalFormat("#.##")
        return "${format.format(bytes / Math.pow(1024.0, exp.toDouble()))} ${pre}B"
    }

    private fun formatDuration(milliseconds: Long): String {
        val seconds = milliseconds / 1000.0
        return String.format("%.1f秒", seconds)
    }

    override fun onDestroy() {
        super.onDestroy()
        monitoringRunnable?.let { handler.removeCallbacks(it) }
        proxy?.stop(5) // 5秒超时
    }
}
```

## 关键集成要点

### 1. 代理配置
- 使用 `localhost:0` 让系统自动分配端口
- 获取实际绑定的地址和端口
- 配置应用的网络库使用这个代理

### 2. 统计监控
- 定期调用统计方法获取最新数据
- 在主线程更新 UI
- 合理的更新频率（建议1秒）

### 3. 资源管理
- 应用退出时停止代理服务
- 取消定时器避免内存泄漏
- 设置合理的停止超时时间

### 4. 错误处理
- 捕获代理启动异常
- 处理网络配置错误
- 提供用户友好的错误信息

### 5. 性能优化
- 统计操作都是轻量级的
- UI 更新在主线程执行
- 避免频繁的统计查询

这些示例展示了如何在真实的移动应用中集成流量监控功能，提供完整的用户体验。