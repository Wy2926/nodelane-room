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
    let bytes = serde_json::to_vec(&request_data)
        .map_err(|_| Failure::new("invalid_request", "请求编码失败"))?;
    if bytes.len() > 65536 {
        return Err(Failure::new("invalid_request", "请求过大"));
    }
    let (_, bytes) = request("POST", "/rpc", bytes, 4 << 20).await?;
    serde_json::from_slice(&bytes).map_err(|_| Failure::new("invalid_response", "服务响应格式无效"))
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
