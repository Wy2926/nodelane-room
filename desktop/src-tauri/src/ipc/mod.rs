mod protocol;
#[cfg(test)]
mod tests;
mod transport;
use base64::{engine::general_purpose::STANDARD, Engine};
use protocol::valid_id;
pub use protocol::{Failure, PlayerRequest};
use serde_json::Value;
use std::sync::Arc;
use tokio::sync::Semaphore;
use transport::request;
pub struct Bridge {
    calls: Arc<Semaphore>,
    images: Arc<Semaphore>,
}

impl Default for Bridge {
    fn default() -> Self {
        Self {
            calls: Arc::new(Semaphore::new(8)),
            images: Arc::new(Semaphore::new(2)),
        }
    }
}

#[tauri::command]
pub async fn player_request(
    state: tauri::State<'_, Bridge>,
    mut request_data: PlayerRequest,
) -> Result<Value, Failure> {
    let _slot = state
        .calls
        .clone()
        .try_acquire_owned()
        .map_err(|_| Failure::new("local_busy", "本机请求繁忙，请稍后重试"))?;
    if (!request_data.room.is_empty() && !valid_id(&request_data.room))
        || request_data.contract != "interaction-1"
        || (!request_data.command_id.is_empty() && !valid_id(&request_data.command_id))
        || request_data.server.len() > 2048
        || request_data.name.len() > 80
        || request_data.target.len() > 128
    {
        return Err(Failure::new("request_validation_failed", "请求字段无效"));
    }
    if request_data.body.is_null() {
        request_data.body = serde_json::json!({});
    }
    if matches!(request_data.action, protocol::Action::Status) {
        request_data.body = serde_json::json!({"gui_version": env!("CARGO_PKG_VERSION")});
    }
    let installing = matches!(request_data.action, protocol::Action::UpdateInstall);
    let login = matches!(
        request_data.action,
        protocol::Action::AccountLogin | protocol::Action::AccountLink
    );
    let bytes = serde_json::to_vec(&request_data)
        .map_err(|_| Failure::new("request_validation_failed", "请求编码失败"))?;
    if bytes.len() > 65536 {
        return Err(Failure::new("request_validation_failed", "请求过大"));
    }
    let (_, bytes) = request("POST", "/rpc", bytes, 4 << 20).await?;
    let value: Value = serde_json::from_slice(&bytes)
        .map_err(|_| Failure::new("local_ipc_response_invalid", "服务响应格式无效"))?;
    if value.get("contract").and_then(Value::as_str) != Some("interaction-1")
        || value.get("request_id").and_then(Value::as_str).is_none()
    {
        return Err(Failure::new("local_ipc_response_invalid", "服务响应不兼容"));
    }
    let data = &value["data"];
    if installing && data.get("elevate").and_then(Value::as_bool) == Some(true) {
        if let Err(error) = launch_updater() {
            let _ = request(
                "POST",
                "/rpc",
                br#"{"contract":"interaction-1","action":"update-cancel-install"}"#.to_vec(),
                65536,
            )
            .await;
            return Err(error);
        }
    }
    if login {
        let url = data
            .get("url")
            .and_then(Value::as_str)
            .ok_or_else(|| Failure::new("local_ipc_response_invalid", "Invalid login response"))?;
        // Only open login URLs obtained from the protected Go service.
        if !valid_login_url(url) {
            return Err(Failure::new(
                "local_ipc_response_invalid",
                "Invalid login URL",
            ));
        }
        open_login_browser(url)?;
    }
    Ok(value)
}

fn launch_updater() -> Result<(), Failure> {
    #[cfg(windows)]
    {
        use windows_sys::Win32::UI::Shell::{
            FOLDERID_ProgramFiles, SHGetKnownFolderPath, ShellExecuteExW, SEE_MASK_FLAG_NO_UI,
            SEE_MASK_NOASYNC, SHELLEXECUTEINFOW,
        };
        let mut folder = std::ptr::null_mut();
        if unsafe {
            SHGetKnownFolderPath(&FOLDERID_ProgramFiles, 0, std::ptr::null_mut(), &mut folder)
        } != 0
        {
            return Err(Failure::new(
                "local_update_install_failed",
                "Could not locate the installed updater",
            ));
        }
        let mut length = 0;
        unsafe {
            while *folder.add(length) != 0 {
                length += 1;
            }
        }
        let base = unsafe { String::from_utf16_lossy(std::slice::from_raw_parts(folder, length)) };
        unsafe {
            windows_sys::Win32::System::Com::CoTaskMemFree(folder.cast());
        }
        let target: Vec<u16> = format!("{base}\\NodeLaneRoom\\nlroom-update.exe")
            .encode_utf16()
            .chain(Some(0))
            .collect();
        let verb: Vec<u16> = "runas".encode_utf16().chain(Some(0)).collect();
        let args: Vec<u16> = "launch".encode_utf16().chain(Some(0)).collect();
        let mut info: SHELLEXECUTEINFOW = unsafe { std::mem::zeroed() };
        info.cbSize = std::mem::size_of::<SHELLEXECUTEINFOW>() as u32;
        info.fMask = SEE_MASK_NOASYNC | SEE_MASK_FLAG_NO_UI;
        info.lpVerb = verb.as_ptr();
        info.lpFile = target.as_ptr();
        info.lpParameters = args.as_ptr();
        if unsafe { ShellExecuteExW(&mut info) } == 0 {
            let cancelled = unsafe { windows_sys::Win32::Foundation::GetLastError() }
                == windows_sys::Win32::Foundation::ERROR_CANCELLED;
            return Err(Failure::new(
                if cancelled {
                    "local_update_elevation_cancelled"
                } else {
                    "local_update_install_failed"
                },
                "Could not start the update helper",
            ));
        }
        Ok(())
    }
    #[cfg(not(windows))]
    {
        Err(Failure::new(
            "local_update_install_failed",
            "Unexpected elevation request",
        ))
    }
}

fn valid_login_url(value: &str) -> bool {
    if value.len() > 4096 || value.chars().any(char::is_control) {
        return false;
    }
    let Ok(url) = tauri::Url::parse(value) else {
        return false;
    };
    let loopback = url.host_str().is_some_and(|host| {
        host == "localhost"
            || host
                .trim_matches(['[', ']'])
                .parse::<std::net::IpAddr>()
                .is_ok_and(|ip| ip.is_loopback())
    });
    let query: Vec<_> = url.query_pairs().collect();
    (url.scheme() == "https" || (url.scheme() == "http" && loopback))
        && url.host_str().is_some()
        && url.username().is_empty()
        && url.password().is_none()
        && url.fragment().is_none()
        && url.path() == "/v2/auth/oidc/browser"
        && query.len() == 1
        && query[0].0 == "id"
        && query[0].1.len() == 32
        && query[0].1.bytes().all(|c| c.is_ascii_hexdigit())
}

fn open_login_browser(url: &str) -> Result<(), Failure> {
    #[cfg(windows)]
    {
        use std::os::windows::ffi::OsStrExt;
        let target: Vec<u16> = std::ffi::OsStr::new(url)
            .encode_wide()
            .chain(Some(0))
            .collect();
        let verb: Vec<u16> = "open".encode_utf16().chain(Some(0)).collect();
        let result = unsafe {
            windows_sys::Win32::UI::Shell::ShellExecuteW(
                std::ptr::null_mut(),
                verb.as_ptr(),
                target.as_ptr(),
                std::ptr::null(),
                std::ptr::null(),
                1,
            )
        };
        if result as isize <= 32 {
            return Err(Failure::new(
                "local_browser_open_failed",
                "Could not open the system browser",
            ));
        }
    }
    #[cfg(unix)]
    {
        std::process::Command::new("xdg-open")
            .arg(url)
            .stdin(std::process::Stdio::null())
            .stdout(std::process::Stdio::null())
            .stderr(std::process::Stdio::null())
            .spawn()
            .map_err(|_| {
                Failure::new(
                    "local_browser_open_failed",
                    "Could not open the system browser",
                )
            })?;
    }
    Ok(())
}

#[tauri::command]
pub async fn game_image(
    state: tauri::State<'_, Bridge>,
    game: String,
    kind: String,
) -> Result<String, Failure> {
    if !valid_id(&game) || (kind != "cover" && kind != "background") {
        return Err(Failure::new("request_validation_failed", "图片标识无效"));
    }
    let _slot = state
        .images
        .clone()
        .try_acquire_owned()
        .map_err(|_| Failure::new("local_busy", "图片请求繁忙"))?;
    let (mime, bytes) = request(
        "GET",
        &format!("/game-images/{game}/{kind}"),
        vec![],
        5 << 20,
    )
    .await?;
    if mime != "image/png" && mime != "image/jpeg" {
        return Err(Failure::new("local_image_unavailable", "图片类型无效"));
    }
    Ok(format!("data:{mime};base64,{}", STANDARD.encode(bytes)))
}
