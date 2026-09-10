#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod ipc;
use std::sync::atomic::{AtomicBool, Ordering};
use tauri::{
    menu::{Menu, MenuItem},
    tray::TrayIconBuilder,
    Emitter, Manager,
};
use tauri_plugin_notification::NotificationExt;

static HAS_TRAY: AtomicBool = AtomicBool::new(false);

fn show(app: &tauri::AppHandle) {
    if let Some(window) = app.get_webview_window("main") {
        let _ = window.show();
        let _ = window.unminimize();
        let _ = window.set_focus();
    }
}

#[tauri::command]
fn exit_app(app: tauri::AppHandle) {
    app.exit(0);
}

#[tauri::command]
fn notify_state(app: tauri::AppHandle, kind: String) -> Result<(), ipc::Failure> {
    let body = match kind.as_str() {
        "disconnected" => "游戏连接已停止，请打开客户端查看状态。",
        "disabled" => "当前游戏已被停用，游戏端口正在撤回。",
        "control_unavailable" => "控制服务暂时不可达，已有连接受当前授权有效期限制。",
        _ => return Err(ipc::Failure::new("invalid_request", "通知类型无效")),
    };
    app.notification()
        .builder()
        .title("NodeLane Room")
        .body(body)
        .show()
        .map_err(|_| ipc::Failure::new("notification_unavailable", "系统通知不可用"))
}

fn main() {
    #[cfg(windows)]
    if unsafe { windows_sys::Win32::UI::Shell::IsUserAnAdmin() } != 0 {
        eprintln!("Run NodeLane Room as the installation user, without administrator elevation.");
        return;
    }
    #[cfg(unix)]
    if unsafe { libc::geteuid() } == 0 {
        eprintln!("Run NodeLane Room as the installation user, without sudo.");
        return;
    }
    tauri::Builder::default()
        .plugin(tauri_plugin_single_instance::init(|app, _, _| show(app)))
        .plugin(tauri_plugin_autostart::Builder::new().build())
        .plugin(tauri_plugin_clipboard_manager::init())
        .plugin(tauri_plugin_notification::init())
        .manage(ipc::Bridge::default())
        .invoke_handler(tauri::generate_handler![
            ipc::player_request,
            ipc::game_image,
            exit_app,
            notify_state
        ])
        .setup(|app| {
            let open = MenuItem::with_id(app, "open", "打开 NodeLane Room", true, None::<&str>)?;
            let leave = MenuItem::with_id(app, "leave", "离房并退出…", true, None::<&str>)?;
            let quit = MenuItem::with_id(app, "quit", "退出界面（继续联机）", true, None::<&str>)?;
            let menu = Menu::with_items(app, &[&open, &leave, &quit])?;
            let icon = app.default_window_icon().cloned();
            let mut tray = TrayIconBuilder::with_id("room")
                .tooltip("NodeLane Room")
                .menu(&menu)
                .on_menu_event(|app, event| match event.id.as_ref() {
                    "open" => show(app),
                    "leave" => {
                        show(app);
                        let _ = app.emit("leave-and-exit", ());
                    }
                    "quit" => app.exit(0),
                    _ => (),
                });
            if let Some(icon) = icon {
                tray = tray.icon(icon);
            }
            if tray.build(app).is_ok() {
                HAS_TRAY.store(true, Ordering::Relaxed);
            }
            // Some Linux desktops do not display application indicators; keep
            // their window discoverable instead of hiding behind an absent tray.
            #[cfg(target_os = "linux")]
            HAS_TRAY.store(false, Ordering::Relaxed);
            Ok(())
        })
        .on_window_event(|window, event| {
            if let tauri::WindowEvent::CloseRequested { api, .. } = event {
                api.prevent_close();
                if HAS_TRAY.load(Ordering::Relaxed) {
                    let _ = window.hide();
                } else {
                    let _ = window.minimize();
                }
            }
        })
        .run(tauri::generate_context!())
        .expect("NodeLane Room could not start");
}
