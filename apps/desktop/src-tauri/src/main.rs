// Argus desktop shell entry point.
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use tauri::Manager;
use std::process::Command;
use std::sync::Mutex;
use std::path::PathBuf;

struct GoServiceState {
    process: Mutex<Option<std::process::Child>>,
}

fn resolve_agent_manager_binary() -> Result<PathBuf, String> {
    let binary_name = if cfg!(windows) { "agent-manager.exe" } else { "agent-manager" };

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

#[tauri::command]
fn start_go_service(state: tauri::State<'_, GoServiceState>) -> Result<String, String> {
    let mut process_guard = state.process.lock().unwrap();
    
    if process_guard.is_some() {
        return Ok("Go service already running".to_string());
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

    let child = cmd.spawn().map_err(|e| {
        let msg = format!("Failed to start Go service: {}", e);
        eprintln!("[sidecar] {}", msg);
        msg
    })?;
    
    *process_guard = Some(child);
    
    Ok("Go service started".to_string())
}

#[tauri::command]
fn stop_go_service(state: tauri::State<'_, GoServiceState>) -> Result<String, String> {
    let mut process_guard = state.process.lock().unwrap();
    
    if let Some(mut child) = process_guard.take() {
        child.kill().map_err(|e| format!("Failed to stop Go service: {}", e))?;
        Ok("Go service stopped".to_string())
    } else {
        Ok("Go service not running".to_string())
    }
}

fn main() {
    tauri::Builder::default()
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