#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod ipc;
mod language;
use language::Language;
use std::sync::{
    atomic::{AtomicBool, Ordering},
    Mutex,
};
use tauri::{
    menu::{Menu, MenuItem},
    tray::TrayIconBuilder,
    Emitter, Manager,
};
use tauri_plugin_notification::NotificationExt;

static HAS_TRAY: AtomicBool = AtomicBool::new(false);

#[derive(Default)]
struct DesktopLanguage(Mutex<Language>);

struct DesktopMenus {
    open: MenuItem<tauri::Wry>,
    leave: MenuItem<tauri::Wry>,
    quit: MenuItem<tauri::Wry>,
}

#[tauri::command]
fn set_language(app: tauri::AppHandle, language: Language) -> Result<(), ipc::Failure> {
    let state = app.state::<DesktopLanguage>();
    *state.0.lock().unwrap_or_else(|error| error.into_inner()) = language;
    if let Some(menu) = app.try_state::<DesktopMenus>() {
        for (item, key) in [
            (&menu.open, "native.open"),
            (&menu.leave, "native.leave"),
            (&menu.quit, "native.quit"),
        ] {
            item.set_text(language.text(key))
                .map_err(|_| ipc::Failure::new("operation_failed", "Tray menu update failed"))?;
        }
    }
    Ok(())
}

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
    let key = match kind.as_str() {
        "disconnected" => "native.disconnected",
        "disabled" => "native.disabled",
        "control_unavailable" => "native.controlUnavailable",
        _ => {
            return Err(ipc::Failure::new(
                "invalid_request",
                "Invalid notification kind",
            ))
        }
    };
    let language = *app
        .state::<DesktopLanguage>()
        .0
        .lock()
        .unwrap_or_else(|error| error.into_inner());
    app.notification()
        .builder()
        .title("NodeLane Room")
        .body(language.text(key))
        .show()
        .map_err(|_| {
            ipc::Failure::new(
                "notification_unavailable",
                "System notifications unavailable",
            )
        })
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
        .manage(DesktopLanguage::default())
        .invoke_handler(tauri::generate_handler![
            ipc::player_request,
            ipc::game_image,
            exit_app,
            notify_state,
            set_language
        ])
        .setup(|app| {
            let language = Language::default();
            let open = MenuItem::with_id(
                app,
                "open",
                language.text("native.open"),
                true,
                None::<&str>,
            )?;
            let leave = MenuItem::with_id(
                app,
                "leave",
                language.text("native.leave"),
                true,
                None::<&str>,
            )?;
            let quit = MenuItem::with_id(
                app,
                "quit",
                language.text("native.quit"),
                true,
                None::<&str>,
            )?;
            let menu = Menu::with_items(app, &[&open, &leave, &quit])?;
            app.manage(DesktopMenus { open, leave, quit });
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
