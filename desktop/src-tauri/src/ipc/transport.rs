#[cfg(windows)]
use super::protocol::valid_id;
use super::protocol::Failure;
use http_body_util::{BodyExt, Full, Limited};
use hyper::{body::Bytes, client::conn::http1, Request};
use hyper_util::rt::TokioIo;
use std::time::Duration;
use tokio::io::{AsyncRead, AsyncWrite};
struct ConnectionTask(tokio::task::JoinHandle<()>);
impl Drop for ConnectionTask {
    fn drop(&mut self) {
        self.0.abort();
    }
}

#[cfg(windows)]
async fn connect() -> Result<tokio::net::windows::named_pipe::NamedPipeClient, Failure> {
    let mut path = String::from(r"\\.\pipe\NodeLaneRoom");
    if cfg!(debug_assertions) {
        if let Ok(test) = std::env::var("NODELANE_TEST_PIPE") {
            if test.starts_with("NodeLaneRoom-Test-") && valid_id(&test) {
                path = format!(r"\\.\pipe\{test}");
            }
        }
    }
    loop {
        match tokio::net::windows::named_pipe::ClientOptions::new().open(&path) {
            Ok(pipe) => return Ok(pipe),
            Err(e) if e.raw_os_error() == Some(231) => {
                tokio::time::sleep(Duration::from_millis(50)).await
            }
            Err(e) => {
                return Err(Failure::new(
                    if e.kind() == std::io::ErrorKind::PermissionDenied {
                        "permission_denied"
                    } else {
                        "service_unavailable"
                    },
                    "无法访问 NodeLane 服务，请检查安装与当前用户权限",
                ))
            }
        }
    }
}

#[cfg(unix)]
async fn connect() -> Result<tokio::net::UnixStream, Failure> {
    let stream = tokio::net::UnixStream::connect("/run/nlroom/agent.sock")
        .await
        .map_err(|e| {
            Failure::new(
                if e.kind() == std::io::ErrorKind::PermissionDenied {
                    "permission_denied"
                } else {
                    "service_unavailable"
                },
                "无法访问 NodeLane 服务，请检查安装与当前用户权限",
            )
        })?;
    if stream
        .peer_cred()
        .map_err(|_| Failure::new("permission_denied", "无法验证本机服务身份"))?
        .uid()
        != 0
    {
        return Err(Failure::new(
            "permission_denied",
            "本机服务必须由 root 运行",
        ));
    }
    Ok(stream)
}

pub(super) async fn exchange<S>(
    stream: S,
    method: &str,
    path: &str,
    body: Vec<u8>,
    limit: usize,
) -> Result<(String, Bytes), Failure>
where
    S: AsyncRead + AsyncWrite + Unpin + Send + 'static,
{
    let (mut sender, connection) = http1::handshake(TokioIo::new(stream))
        .await
        .map_err(|_| Failure::new("service_unavailable", "本机服务握手失败"))?;
    // The connection task owns its stream and exits when this one request finishes.
    let task = ConnectionTask(tokio::spawn(async move {
        let _ = connection.await;
    }));
    let result = async {
        let req = Request::builder()
            .method(method)
            .uri(path)
            .header("Host", "nodelane")
            .header("Content-Type", "application/json")
            .header("Connection", "close")
            .body(Full::new(Bytes::from(body)))
            .map_err(|_| Failure::new("invalid_request", "本机请求无效"))?;
        let response = sender
            .send_request(req)
            .await
            .map_err(|_| Failure::new("service_unavailable", "本机服务连接中断"))?;
        let status = response.status();
        let content_type = response
            .headers()
            .get("content-type")
            .and_then(|v| v.to_str().ok())
            .unwrap_or("")
            .to_owned();
        let bytes = Limited::new(response.into_body(), limit)
            .collect()
            .await
            .map_err(|_| Failure::new("invalid_response", "本机服务响应无效或过大"))?
            .to_bytes();
        if !status.is_success() {
            return Err(
                serde_json::from_slice::<Failure>(&bytes).unwrap_or_else(|_| {
                    Failure::new("operation_failed", "本机操作失败，请检查客户端与服务版本")
                }),
            );
        }
        Ok((content_type, bytes))
    }
    .await;
    drop(task);
    result
}

pub(super) async fn request(
    method: &str,
    path: &str,
    body: Vec<u8>,
    limit: usize,
) -> Result<(String, Bytes), Failure> {
    tokio::time::timeout(Duration::from_secs(45), async {
        exchange(connect().await?, method, path, body, limit).await
    })
    .await
    .map_err(|_| Failure::new("timeout", "请求超时；操作结果可能尚未确认，请刷新状态"))?
}
