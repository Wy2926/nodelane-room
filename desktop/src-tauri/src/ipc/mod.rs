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
        .map_err(|_| Failure::new("busy", "本机请求繁忙，请稍后重试"))?;
    if (!request_data.room.is_empty() && !valid_id(&request_data.room))
        || request_data.server.len() > 2048
        || request_data.name.len() > 80
        || request_data.target.len() > 128
    {
        return Err(Failure::new("invalid_request", "请求字段无效"));
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
        .map_err(|_| Failure::new("invalid_request", "请求编码失败"))?;
    if bytes.len() > 65536 {
        return Err(Failure::new("invalid_request", "请求过大"));
    }
    let (_, bytes) = request("POST", "/rpc", bytes, 4 << 20).await?;
    let value: Value = serde_json::from_slice(&bytes)
        .map_err(|_| Failure::new("invalid_response", "服务响应格式无效"))?;
    if installing && value.get("elevate").and_then(Value::as_bool) == Some(true) {
        if let Err(error) = launch_updater() {
            let _ = request(
                "POST",
                "/rpc",
                br#"{"action":"update-cancel-install"}"#.to_vec(),
                65536,
            )
            .await;
            return Err(error);
        }
    }
    if login {
        let url = value
            .get("url")
            .and_then(Value::as_str)
            .ok_or_else(|| Failure::new("invalid_response", "Invalid login response"))?;
        // Only open login URLs obtained from the protected Go service.
        if !valid_login_url(url) {
            return Err(Failure::new("invalid_response", "Invalid login URL"));
        }
        open_login_browser(url)?;
    }
    Ok(value)
}

fn launch_updater() -> Result<(), Failure> {
    #[cfg(windows)]
    {
        use windows_sys::Win32::UI::Shell::{
            FOLDERID_ProgramFiles, SHGetKnownFolderPath, ShellExecuteW,
        };
        let mut folder = std::ptr::null_mut();
        if unsafe {
            SHGetKnownFolderPath(&FOLDERID_ProgramFiles, 0, std::ptr::null_mut(), &mut folder)
        } != 0
        {
            return Err(Failure::new(
                "update_install_failed",
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
        let result = unsafe {
            ShellExecuteW(
                std::ptr::null_mut(),
                verb.as_ptr(),
                target.as_ptr(),
                args.as_ptr(),
                std::ptr::null(),
                0,
            )
        };
        if result as isize <= 32 {
            return Err(Failure::new(
                "update_install_failed",
                "Update elevation was cancelled or failed",
            ));
        }
        Ok(())
    }
    #[cfg(not(windows))]
    {
        Err(Failure::new(
            "update_install_failed",
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
                "operation_failed",
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
            .map_err(|_| Failure::new("operation_failed", "Could not open the system browser"))?;
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
        return Err(Failure::new("invalid_request", "图片标识无效"));
    }
    let _slot = state
        .images
        .clone()
        .try_acquire_owned()
        .map_err(|_| Failure::new("busy", "图片请求繁忙"))?;
    let (mime, bytes) = request(
        "GET",
        &format!("/game-images/{game}/{kind}"),
        vec![],
        5 << 20,
    )
    .await?;
    if mime != "image/png" && mime != "image/jpeg" {
        return Err(Failure::new("image_invalid", "图片类型无效"));
    }
    Ok(format!("data:{mime};base64,{}", STANDARD.encode(bytes)))
}
