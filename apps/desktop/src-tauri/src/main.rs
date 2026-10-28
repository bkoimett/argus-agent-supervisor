// Argus desktop shell entry point.
// TODO (M1): wire this up once the Go service exposes a local health
// check endpoint, per WORKFLOW.md M1 checkpoint.
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

fn main() {
    tauri::Builder::default()
        .run(tauri::generate_context!())
        .expect("error while running argus desktop shell");
}
