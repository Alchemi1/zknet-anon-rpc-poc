mod utils;

use serde::Serialize;
use std::process::Command;

#[derive(Serialize)]
struct ContainerInfo {
    name: String,
    status: String,
    ports: String,
}

#[derive(Serialize)]
struct EpochInfo {
    current_epoch: String,
    consensus_status: String,
    last_consensus: String,
}

#[derive(Serialize)]
struct ProxyTestResult {
    success: bool,
    response: String,
    duration_ms: u64,
    error: String,
}

#[derive(Serialize)]
struct LogEntry {
    container: String,
    line: String,
}

fn get_container_status() -> Vec<ContainerInfo> {
    let mut containers = Vec::new();
    let names = vec![
        "mix-dirauth-1", "mix-dirauth-2", "mix-dirauth-3",
        "mix-1", "mix-2", "mix-3",
        "mix-gateway", "mix-servicenode", "mix-client",
    ];

    for name in &names {
        let output = Command::new("docker")
            .args(["ps", "--filter", &format!("name={}", name), "--format", "{{.Names}}\t{{.Status}}\t{{.Ports}}"])
            .output();
        match output {
            Ok(out) => {
                let line = String::from_utf8_lossy(&out.stdout).to_string();
                let parts: Vec<&str> = line.trim().split('\t').collect();
                if parts.len() >= 2 {
                    containers.push(ContainerInfo {
                        name: parts[0].to_string(),
                        status: parts[1].to_string(),
                        ports: if parts.len() >= 3 { parts[2].to_string() } else { String::new() },
                    });
                } else {
                    containers.push(ContainerInfo {
                        name: name.to_string(),
                        status: "down".to_string(),
                        ports: String::new(),
                    });
                }
            }
            Err(_) => {
                containers.push(ContainerInfo {
                    name: name.to_string(),
                    status: "error".to_string(),
                    ports: String::new(),
                });
            }
        }
    }
    containers
}

#[tauri::command]
fn get_containers() -> Vec<ContainerInfo> {
    get_container_status()
}

#[tauri::command]
fn get_epoch_status() -> EpochInfo {
    let log_paths = vec![
        "/var/lib/katzenpost/auth1/katzenpost.log",
        "/var/lib/katzenpost/auth2/katzenpost.log",
        "/var/lib/katzenpost/auth3/katzenpost.log",
    ];

    let mut last_consensus = String::new();
    let mut current_epoch = String::new();
    let mut consensus_ok = 0;

    for path in &log_paths {
        if let Ok(lines) = utils::tail_file(path, 200) {
            for line in &lines {
                if line.contains("SUCCESS! Achieved threshold consensus") && line.contains("Epoch:") {
                    let parts: Vec<&str> = line.split("Epoch: ").collect();
                    if parts.len() >= 2 {
                        let epoch_str = parts[1].split_whitespace().next().unwrap_or("");
                        last_consensus = format!("Epoch {}", epoch_str);
                        consensus_ok += 1;
                    }
                }
            }
        }
    }

    // Try to get current epoch from client log
    if let Ok(lines) = utils::tail_file("/var/lib/katzenpost/client/client.log", 100) {
        for line in &lines {
            if line.contains("Cached PKI document for epoch") {
                let parts: Vec<&str> = line.split("epoch ").collect();
                if parts.len() >= 2 {
                    current_epoch = parts[1].trim().to_string();
                }
            }
        }
    }

    let status = if consensus_ok >= 3 {
        "consensus OK".to_string()
    } else if consensus_ok > 0 {
        format!("partial consensus ({}/3)", consensus_ok)
    } else {
        "no consensus".to_string()
    };

    EpochInfo {
        current_epoch,
        consensus_status: status,
        last_consensus,
    }
}

#[tauri::command]
fn test_http_proxy(url: String) -> ProxyTestResult {
    let start = std::time::Instant::now();
    let output = Command::new("curl")
        .args([
            "-s", "--max-time", "120",
            "-X", "POST",
            &url,
            "-H", "Content-Type: application/json",
            "-H", "Host: ethereum-sepolia.publicnode.com",
            "-d", r#"{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}"#,
        ])
        .output();

    let duration = start.elapsed().as_millis() as u64;

    match output {
        Ok(out) => {
            let stdout = String::from_utf8_lossy(&out.stdout).to_string();
            let stderr = String::from_utf8_lossy(&out.stderr).to_string();
            if stdout.contains("result") {
                ProxyTestResult {
                    success: true,
                    response: stdout,
                    duration_ms: duration,
                    error: String::new(),
                }
            } else if !stdout.is_empty() {
                ProxyTestResult {
                    success: false,
                    response: stdout,
                    duration_ms: duration,
                    error: stderr,
                }
            } else {
                ProxyTestResult {
                    success: false,
                    response: String::new(),
                    duration_ms: duration,
                    error: if stderr.is_empty() { "timeout or no response".to_string() } else { stderr },
                }
            }
        }
        Err(e) => ProxyTestResult {
            success: false,
            response: String::new(),
            duration_ms: duration,
            error: format!("curl failed: {}", e),
        },
    }
}

#[tauri::command]
fn get_container_logs(container: String, tail: usize) -> Vec<String> {
    let output = Command::new("docker")
        .args(["logs", &container, "--tail", &tail.to_string()])
        .output();
    match output {
        Ok(out) => {
            String::from_utf8_lossy(&out.stdout)
                .lines()
                .map(|l| l.to_string())
                .collect()
        }
        Err(_) => vec![format!("(error fetching logs for {})", container)],
    }
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .invoke_handler(tauri::generate_handler![
            get_containers,
            get_epoch_status,
            test_http_proxy,
            get_container_logs,
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
