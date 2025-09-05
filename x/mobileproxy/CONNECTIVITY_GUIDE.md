# MobileProxy HTTP Connectivity Testing Integration Guide

This guide provides comprehensive examples for integrating HTTP connectivity testing into iOS applications using MobileProxy's new connectivity testing functionality.

## Overview

MobileProxy's HTTP connectivity testing provides:
- Single configuration testing with detailed error reporting
- Error codes compatible with Outline app for consistent user experience  
- JSON-based results for easy parsing and handling
- Optional testing (pass empty URL to skip)
- Performance metrics including connection timing

## Swift Integration

### Basic Swift Implementation

```swift
import Foundation
import Mobileproxy

class ConnectivityTester {
    
    struct ConnectivityResult {
        let success: Bool
        let durationMs: Int
        let errorCode: String?
        let errorMessage: String?
        let config: String
        let testURL: String?
    }
    
    func testConnectivity(accessKey: String, testURL: String?) -> ConnectivityResult {
        do {
            // Create dialer from access key
            let dialer = try MobileproxyNewStreamDialerFromConfig(accessKey)
            
            // Perform connectivity test (use empty string to skip)
            let testURLString = testURL ?? ""
            let resultJSON = MobileproxyTestHTTPConnectivity(dialer, testURLString)
            
            // Parse JSON result
            return parseResult(json: resultJSON)
            
        } catch {
            return ConnectivityResult(
                success: false,
                durationMs: 0,
                errorCode: "ERR_ILLEGAL_CONFIG",
                errorMessage: "Failed to create dialer: \(error.localizedDescription)",
                config: accessKey,
                testURL: testURL
            )
        }
    }
    
    private func parseResult(json: String) -> ConnectivityResult {
        guard let data = json.data(using: .utf8),
              let parsed = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else {
            return ConnectivityResult(
                success: false,
                durationMs: 0,
                errorCode: "ERR_INTERNAL_ERROR",
                errorMessage: "Failed to parse result JSON",
                config: "unknown",
                testURL: nil
            )
        }
        
        let success = parsed["success"] as? Bool ?? false
        let durationMs = parsed["durationMs"] as? Int ?? 0
        let config = parsed["config"] as? String ?? "unknown"
        let testURL = parsed["testURL"] as? String
        
        var errorCode: String?
        var errorMessage: String?
        
        if let error = parsed["error"] as? [String: Any] {
            errorCode = error["code"] as? String
            errorMessage = error["message"] as? String
        }
        
        return ConnectivityResult(
            success: success,
            durationMs: durationMs,
            errorCode: errorCode,
            errorMessage: errorMessage,
            config: config,
            testURL: testURL
        )
    }
}
```

### Advanced Swift Usage with Async/Await

```swift
import Foundation
import Mobileproxy

@available(iOS 13.0, *)
class AsyncConnectivityTester {
    
    func testConnectivity(accessKey: String, testURL: String?) async -> ConnectivityTester.ConnectivityResult {
        return await withCheckedContinuation { continuation in
            DispatchQueue.global(qos: .userInitiated).async {
                let tester = ConnectivityTester()
                let result = tester.testConnectivity(accessKey: accessKey, testURL: testURL)
                continuation.resume(returning: result)
            }
        }
    }
    
    func testMultipleConfigurations(_ accessKeys: [String], testURL: String?) async -> [ConnectivityTester.ConnectivityResult] {
        return await withTaskGroup(of: ConnectivityTester.ConnectivityResult.self, returning: [ConnectivityTester.ConnectivityResult].self) { group in
            
            for accessKey in accessKeys {
                group.addTask {
                    await self.testConnectivity(accessKey: accessKey, testURL: testURL)
                }
            }
            
            var results: [ConnectivityTester.ConnectivityResult] = []
            for await result in group {
                results.append(result)
            }
            
            // Sort by success and then by latency
            return results.sorted { lhs, rhs in
                if lhs.success != rhs.success {
                    return lhs.success && !rhs.success
                }
                return lhs.durationMs < rhs.durationMs
            }
        }
    }
}
```

## Objective-C Integration

### Basic Objective-C Implementation

```objc
// ConnectivityTester.h
@import Foundation;

NS_ASSUME_NONNULL_BEGIN

@interface ConnectivityResult : NSObject
@property (nonatomic, readonly) BOOL success;
@property (nonatomic, readonly) NSInteger durationMs;
@property (nonatomic, readonly, nullable) NSString *errorCode;
@property (nonatomic, readonly, nullable) NSString *errorMessage;
@property (nonatomic, readonly) NSString *config;
@property (nonatomic, readonly, nullable) NSString *testURL;

- (instancetype)initWithSuccess:(BOOL)success
                     durationMs:(NSInteger)durationMs
                      errorCode:(nullable NSString *)errorCode
                   errorMessage:(nullable NSString *)errorMessage
                         config:(NSString *)config
                        testURL:(nullable NSString *)testURL;
@end

@interface ConnectivityTester : NSObject
- (ConnectivityResult *)testConnectivity:(NSString *)accessKey testURL:(nullable NSString *)testURL;
- (NSString *)userFriendlyErrorMessage:(NSString *)errorCode;
@end

NS_ASSUME_NONNULL_END

// ConnectivityTester.m
#import "ConnectivityTester.h"
#import "Mobileproxy.objc.h"

@implementation ConnectivityResult

- (instancetype)initWithSuccess:(BOOL)success
                     durationMs:(NSInteger)durationMs
                      errorCode:(NSString *)errorCode
                   errorMessage:(NSString *)errorMessage
                         config:(NSString *)config
                        testURL:(NSString *)testURL {
    if (self = [super init]) {
        _success = success;
        _durationMs = durationMs;
        _errorCode = errorCode;
        _errorMessage = errorMessage;
        _config = config;
        _testURL = testURL;
    }
    return self;
}

@end

@implementation ConnectivityTester

- (ConnectivityResult *)testConnectivity:(NSString *)accessKey testURL:(NSString *)testURL {
    NSError *error;
    
    // Create dialer from access key
    MobileproxyStreamDialer *dialer = MobileproxyNewStreamDialerFromConfig(accessKey, &error);
    if (error) {
        return [[ConnectivityResult alloc] initWithSuccess:NO
                                                durationMs:0
                                                 errorCode:@"ERR_ILLEGAL_CONFIG"
                                              errorMessage:[NSString stringWithFormat:@"Failed to create dialer: %@", error.localizedDescription]
                                                    config:accessKey
                                                   testURL:testURL];
    }
    
    // Perform connectivity test
    NSString *testURLString = testURL ?: @"";
    NSString *resultJSON = MobileproxyTestHTTPConnectivity(dialer, testURLString);
    
    // Parse JSON result
    return [self parseResult:resultJSON config:accessKey];
}

- (ConnectivityResult *)parseResult:(NSString *)json config:(NSString *)config {
    NSData *data = [json dataUsingEncoding:NSUTF8StringEncoding];
    if (!data) {
        return [[ConnectivityResult alloc] initWithSuccess:NO
                                                durationMs:0
                                                 errorCode:@"ERR_INTERNAL_ERROR"
                                              errorMessage:@"Failed to parse result JSON"
                                                    config:config
                                                   testURL:nil];
    }
    
    NSError *parseError;
    NSDictionary *parsed = [NSJSONSerialization JSONObjectWithData:data options:0 error:&parseError];
    if (parseError) {
        return [[ConnectivityResult alloc] initWithSuccess:NO
                                                durationMs:0
                                                 errorCode:@"ERR_INTERNAL_ERROR"
                                              errorMessage:[NSString stringWithFormat:@"JSON parse error: %@", parseError.localizedDescription]
                                                    config:config
                                                   testURL:nil];
    }
    
    BOOL success = [parsed[@"success"] boolValue];
    NSInteger durationMs = [parsed[@"durationMs"] integerValue];
    NSString *testURL = parsed[@"testURL"];
    
    NSString *errorCode = nil;
    NSString *errorMessage = nil;
    
    if (!success && parsed[@"error"]) {
        NSDictionary *errorInfo = parsed[@"error"];
        errorCode = errorInfo[@"code"];
        errorMessage = errorInfo[@"message"];
    }
    
    return [[ConnectivityResult alloc] initWithSuccess:success
                                            durationMs:durationMs
                                             errorCode:errorCode
                                          errorMessage:errorMessage
                                                config:config
                                               testURL:testURL];
}

- (NSString *)userFriendlyErrorMessage:(NSString *)errorCode {
    static NSDictionary *messages;
    static dispatch_once_t onceToken;
    dispatch_once(&onceToken, ^{
        messages = @{
            @"ERR_PROXY_SERVER_UNREACHABLE": @"Cannot connect to proxy server. Please check your connection and try again.",
            @"ERR_PROXY_SERVER_READ_FAILURE": @"Failed to communicate with proxy server. The server may be experiencing issues.",
            @"ERR_PROXY_SERVER_WRITE_FAILURE": @"Failed to send data to proxy server. Please check your connection.",
            @"ERR_CLIENT_UNAUTHENTICATED": @"Invalid access key. Please check your configuration and try again.",
            @"ERR_TIMEOUT": @"Connection timed out. The server may be slow or unreachable.",
            @"ERR_ILLEGAL_CONFIG": @"Invalid configuration. Please check your access key format.",
            @"ERR_RESOLVE_IP_FAILURE": @"DNS resolution failed. Please check your internet connection.",
            @"ERR_INTERNAL_ERROR": @"An internal error occurred. Please try again."
        };
    });
    
    return messages[errorCode] ?: @"Connection test failed. Please try again.";
}

@end
```

### Async Objective-C Implementation with Completion Handlers

```objc
// AsyncConnectivityTester.h
@import Foundation;
#import "ConnectivityTester.h"

NS_ASSUME_NONNULL_BEGIN

typedef void (^ConnectivityTestCompletion)(ConnectivityResult *result);
typedef void (^MultipleTestCompletion)(NSArray<ConnectivityResult *> *results, ConnectivityResult * _Nullable bestResult);

@interface AsyncConnectivityTester : NSObject

- (void)testConnectivity:(NSString *)accessKey
                 testURL:(nullable NSString *)testURL
              completion:(ConnectivityTestCompletion)completion;

- (void)testMultipleConfigurations:(NSArray<NSString *> *)accessKeys
                           testURL:(nullable NSString *)testURL
                        completion:(MultipleTestCompletion)completion;

- (void)cancelAllTests;

@end

NS_ASSUME_NONNULL_END

// AsyncConnectivityTester.m
#import "AsyncConnectivityTester.h"

@interface AsyncConnectivityTester ()
@property (nonatomic, strong) NSOperationQueue *testQueue;
@end

@implementation AsyncConnectivityTester

- (instancetype)init {
    if (self = [super init]) {
        _testQueue = [[NSOperationQueue alloc] init];
        _testQueue.maxConcurrentOperationCount = 5; // Limit concurrent tests
        _testQueue.qualityOfService = NSQualityOfServiceUserInitiated;
    }
    return self;
}

- (void)testConnectivity:(NSString *)accessKey
                 testURL:(NSString *)testURL
              completion:(ConnectivityTestCompletion)completion {
    
    [self.testQueue addOperationWithBlock:^{
        ConnectivityTester *tester = [[ConnectivityTester alloc] init];
        ConnectivityResult *result = [tester testConnectivity:accessKey testURL:testURL];
        
        dispatch_async(dispatch_get_main_queue(), ^{
            completion(result);
        });
    }];
}

- (void)testMultipleConfigurations:(NSArray<NSString *> *)accessKeys
                           testURL:(NSString *)testURL
                        completion:(MultipleTestCompletion)completion {
    
    NSMutableArray<ConnectivityResult *> *results = [NSMutableArray arrayWithCapacity:accessKeys.count];
    NSInteger totalTests = accessKeys.count;
    __block NSInteger completedTests = 0;
    
    dispatch_group_t group = dispatch_group_create();
    
    for (NSString *accessKey in accessKeys) {
        dispatch_group_enter(group);
        
        [self testConnectivity:accessKey testURL:testURL completion:^(ConnectivityResult *result) {
            @synchronized(results) {
                [results addObject:result];
                completedTests++;
            }
            dispatch_group_leave(group);
        }];
    }
    
    dispatch_group_notify(group, dispatch_get_main_queue(), ^{
        // Sort results: successful tests first, then by latency
        NSArray<ConnectivityResult *> *sortedResults = [results sortedArrayUsingComparator:^NSComparisonResult(ConnectivityResult *lhs, ConnectivityResult *rhs) {
            if (lhs.success != rhs.success) {
                return lhs.success ? NSOrderedAscending : NSOrderedDescending;
            }
            return [@(lhs.durationMs) compare:@(rhs.durationMs)];
        }];
        
        // Find best result (first successful one with lowest latency)
        ConnectivityResult *bestResult = nil;
        for (ConnectivityResult *result in sortedResults) {
            if (result.success) {
                bestResult = result;
                break;
            }
        }
        
        completion(sortedResults, bestResult);
    });
}

- (void)cancelAllTests {
    [self.testQueue cancelAllOperations];
}

@end
```

## Integration with UI

### SwiftUI Example

```swift
import SwiftUI
import Mobileproxy

struct ConnectivityTestView: View {
    @State private var accessKey: String = ""
    @State private var testURL: String = "http://example.com"
    @State private var isTestingConnectivity = false
    @State private var testResult: ConnectivityTester.ConnectivityResult?
    
    private let tester = ConnectivityTester()
    
    var body: some View {
        VStack(spacing: 20) {
            TextField("Access Key", text: $accessKey)
                .textFieldStyle(RoundedBorderTextFieldStyle())
            
            TextField("Test URL (optional)", text: $testURL)
                .textFieldStyle(RoundedBorderTextFieldStyle())
            
            Button(action: testConnectivity) {
                if isTestingConnectivity {
                    HStack {
                        ProgressView()
                            .scaleEffect(0.8)
                        Text("Testing...")
                    }
                } else {
                    Text("Test Connectivity")
                }
            }
            .disabled(accessKey.isEmpty || isTestingConnectivity)
            
            if let result = testResult {
                ConnectivityResultView(result: result)
            }
        }
        .padding()
    }
    
    private func testConnectivity() {
        isTestingConnectivity = true
        testResult = nil
        
        DispatchQueue.global(qos: .userInitiated).async {
            let result = tester.testConnectivity(
                accessKey: accessKey,
                testURL: testURL.isEmpty ? nil : testURL
            )
            
            DispatchQueue.main.async {
                self.testResult = result
                self.isTestingConnectivity = false
            }
        }
    }
}

struct ConnectivityResultView: View {
    let result: ConnectivityTester.ConnectivityResult
    
    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Image(systemName: result.success ? "checkmark.circle.fill" : "xmark.circle.fill")
                    .foregroundColor(result.success ? .green : .red)
                Text(result.success ? "Connected" : "Connection Failed")
                    .font(.headline)
            }
            
            Text("Duration: \(result.durationMs)ms")
                .font(.subheadline)
                .foregroundColor(.secondary)
            
            if let errorCode = result.errorCode, let errorMessage = result.errorMessage {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Error: \(errorCode)")
                        .font(.caption)
                        .foregroundColor(.red)
                    Text(errorMessage)
                        .font(.caption)
                        .foregroundColor(.secondary)
                }
            }
        }
        .padding()
        .background(Color.gray.opacity(0.1))
        .cornerRadius(8)
    }
}
```

### UIKit Example

```objc
// ConnectivityTestViewController.h
@import UIKit;
#import "AsyncConnectivityTester.h"

@interface ConnectivityTestViewController : UIViewController
@end

// ConnectivityTestViewController.m
#import "ConnectivityTestViewController.h"

@interface ConnectivityTestViewController ()
@property (weak, nonatomic) IBOutlet UITextField *accessKeyTextField;
@property (weak, nonatomic) IBOutlet UITextField *testURLTextField;
@property (weak, nonatomic) IBOutlet UIButton *testButton;
@property (weak, nonatomic) IBOutlet UIActivityIndicatorView *activityIndicator;
@property (weak, nonatomic) IBOutlet UILabel *resultLabel;
@property (weak, nonatomic) IBOutlet UIView *resultView;

@property (strong, nonatomic) AsyncConnectivityTester *connectivityTester;
@end

@implementation ConnectivityTestViewController

- (void)viewDidLoad {
    [super viewDidLoad];
    
    self.connectivityTester = [[AsyncConnectivityTester alloc] init];
    self.testURLTextField.text = @"http://example.com";
    [self updateUI];
}

- (IBAction)testConnectivity:(id)sender {
    [self startTest];
    
    NSString *accessKey = self.accessKeyTextField.text;
    NSString *testURL = self.testURLTextField.text.length > 0 ? self.testURLTextField.text : nil;
    
    [self.connectivityTester testConnectivity:accessKey
                                      testURL:testURL
                                   completion:^(ConnectivityResult *result) {
        [self displayResult:result];
    }];
}

- (void)startTest {
    self.testButton.enabled = NO;
    [self.activityIndicator startAnimating];
    self.resultView.hidden = YES;
}

- (void)displayResult:(ConnectivityResult *)result {
    self.testButton.enabled = YES;
    [self.activityIndicator stopAnimating];
    self.resultView.hidden = NO;
    
    if (result.success) {
        self.resultLabel.text = [NSString stringWithFormat:@"✅ Connected in %ldms", (long)result.durationMs];
        self.resultLabel.textColor = [UIColor systemGreenColor];
    } else {
        ConnectivityTester *tester = [[ConnectivityTester alloc] init];
        NSString *userMessage = [tester userFriendlyErrorMessage:result.errorCode];
        self.resultLabel.text = [NSString stringWithFormat:@"❌ %@\nDuration: %ldms", userMessage, (long)result.durationMs];
        self.resultLabel.textColor = [UIColor systemRedColor];
    }
}

- (void)updateUI {
    BOOL hasAccessKey = self.accessKeyTextField.text.length > 0;
    self.testButton.enabled = hasAccessKey && !self.activityIndicator.isAnimating;
}

- (IBAction)accessKeyChanged:(id)sender {
    [self updateUI];
}

@end
```

## Best Practices Summary

1. **Always test on background queues** to avoid blocking the UI
2. **Handle all error codes** with user-friendly messages
3. **Use optional testing** by passing empty/nil URL when testing is not required
4. **Parse JSON results carefully** with proper error handling
5. **Sort multiple test results** by success status and latency
6. **Implement proper cancellation** for long-running batch tests
7. **Monitor performance** using the durationMs metric
8. **Cache successful configurations** for faster subsequent connections

This implementation provides a robust foundation for integrating HTTP connectivity testing into iOS applications using MobileProxy.