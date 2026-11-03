// Argus desktop shell entry point.
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use std::net::{SocketAddr, TcpStream};
use std::path::PathBuf;
use std::process::Command;
use std::sync::Mutex;
use std::thread;
use std::time::{Duration, Instant};
use tauri::Manager;
use tauri_plugin_dialog;

const GO_SERVICE_PORT: u16 = 8080;
const GO_SERVICE_ADDR: &str = "127.0.0.1:8080";

/// Parses the configured Go service address into a SocketAddr.
fn go_service_addr() -> SocketAddr {
    GO_SERVICE_ADDR
        .parse::<SocketAddr>()
        .expect("GO_SERVICE_ADDR must be a valid host:port SocketAddr")
}

struct GoServiceState {
    process: Mutex<Option<std::process::Child>>,
}

fn resolve_agent_manager_binary() -> Result<PathBuf, String> {
    let binary_name = if cfg!(windows) {
        "agent-manager.exe"
    } else {
        "agent-manager"
    };

    // 1. Explicit override via environment variable
    if let Ok(path) = std::env::var("ARGUS_AGENT_MANAGER_PATH") {
        let p = PathBuf::from(path);
        if p.exists() {
            return Ok(p);
        }
    }

    // 2. Next to the current executable (bundled sidecar)
    if let Ok(exe) = std::env::current_exe() {
        if let Some(dir) = exe.parent() {
            let candidate = dir.join(&binary_name);
            if candidate.exists() {
                return Ok(candidate);
            }
        }
    }

    // 3. Development fallback: repo layout (apps/desktop/src-tauri/target/... -> repo root -> services/agent-manager)
    if let Ok(exe) = std::env::current_exe() {
        if let Some(dir) = exe.parent() {
            // target/debug or target/release -> go up 4 levels to repo root
            let candidate = dir
                .join("..") // target/debug -> target
                .join("..") // target -> src-tauri
                .join("..") // src-tauri -> apps/desktop
                .join("..") // apps/desktop -> repo root
                .join("services")
                .join("agent-manager")
                .join(&binary_name);
            if candidate.exists() {
                return Ok(candidate.canonicalize().unwrap_or(candidate));
            }
        }
    }

    Err("Could not locate agent-manager binary. Set ARGUS_AGENT_MANAGER_PATH or build the Go service.".to_string())
}

/// Returns true if something is already listening on the Go service port.
fn port_open(addr: &SocketAddr) -> bool {
    TcpStream::connect_timeout(addr, Duration::from_millis(200)).is_ok()
}

/// Waits for the newly spawned Go service to bind its port, up to timeout_ms.
/// Returns false if the child process exits before binding (e.g. port conflict,
/// which the Go service now treats as fatal).
fn wait_for_ready(child: &mut std::process::Child, timeout_ms: u64) -> bool {
    let deadline = Instant::now() + Duration::from_millis(timeout_ms);
    let interval = Duration::from_millis(100);
    while Instant::now() < deadline {
        match child.try_wait() {
            Ok(Some(_)) => return false, // child exited before binding
            Ok(None) => {}               // still running
            Err(_) => return false,      // already reaped
        }
        if port_open(&go_service_addr()) {
            return true;
        }
        thread::sleep(interval);
    }
    port_open(&go_service_addr())
}

#[tauri::command]
fn start_go_service(state: tauri::State<'_, GoServiceState>) -> Result<String, String> {
    let mut process_guard = state.process.lock().unwrap();

    if process_guard.is_some() {
        return Ok("Go service already running".to_string());
    }

    // Pre-flight: refuse to silently attach to a stale service already
    // listening on 8080. The user must stop it first.
    if port_open(&go_service_addr()) {
        return Err(format!(
            "Go service appears to already be running on port {}. Stop it before starting Argus.",
            GO_SERVICE_PORT
        ));
    }

    let binary_path = match resolve_agent_manager_binary() {
        Ok(p) => p,
        Err(e) => {
            eprintln!("[sidecar] resolve error: {}", e);
            return Err(e);
        }
    };

    eprintln!("[sidecar] starting {}", binary_path.display());
    let mut cmd = Command::new(&binary_path);
    cmd.args(["--port=8080", "--db=argus.db"]);
    // Inherit stdout/stderr so we see logs in the terminal
    cmd.stdout(std::process::Stdio::inherit());
    cmd.stderr(std::process::Stdio::inherit());

    let mut child = match cmd.spawn() {
        Ok(c) => c,
        Err(e) => {
            let msg = format!("Failed to start Go service: {}", e);
            eprintln!("[sidecar] {}", msg);
            return Err(msg);
        }
    };

    // Readiness poll: the new sidecar must bind 8080 within the timeout. If it
    // exits first (the Go service now exits non-zero on a bind conflict),
    // surface the failure instead of leaving the UI to talk to a stale service.
    if !wait_for_ready(&mut child, 5_000) {
        let _ = child.kill();
        let _ = child.wait();
        return Err("Go service failed to start on port 8080 within the timeout. Is another instance already running?".to_string());
    }

    eprintln!("[sidecar] Go service ready on port {}", GO_SERVICE_PORT);
    *process_guard = Some(child);

    Ok("Go service started".to_string())
}

#[tauri::command]
fn stop_go_service(state: tauri::State<'_, GoServiceState>) -> Result<String, String> {
    let mut process_guard = state.process.lock().unwrap();

    if let Some(mut child) = process_guard.take() {
        child
            .kill()
            .map_err(|e| format!("Failed to stop Go service: {}", e))?;
        let _ = child.wait();
        Ok("Go service stopped".to_string())
    } else {
        Ok("Go service not running".to_string())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::net::SocketAddr;

    #[test]
    fn test_go_service_addr_parses() {
        let addr = go_service_addr();
        assert_eq!(addr, SocketAddr::from(([127, 0, 0, 1], 8080)));
        assert_eq!(addr.port(), GO_SERVICE_PORT);
    }

    #[test]
    fn test_port_open_unused_port_is_false() {
        // A port extremely unlikely to be in use; connect_timeout should fail
        // fast with connection refused rather than silently succeeding.
        let unused: SocketAddr = "127.0.0.1:61111".parse().expect("valid test addr");
        assert!(!port_open(&unused));
    }
}

fn main() {
    tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .manage(GoServiceState {
            process: Mutex::new(None),
        })
        .invoke_handler(tauri::generate_handler![start_go_service, stop_go_service])
        .setup(|app| {
            // Auto-start Go service on app launch
            let state: tauri::State<'_, GoServiceState> = app.state();
            if let Err(e) = start_go_service(state) {
                eprintln!("[sidecar] auto-start failed: {}", e);
            }
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running argus desktop shell");
}
