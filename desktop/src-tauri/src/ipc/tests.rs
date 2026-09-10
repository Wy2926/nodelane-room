use super::transport::exchange;
use super::*;

// Run only against an explicitly provisioned disposable/installed Go service.
// Unlike the duplex tests, this uses the production pipe/socket transport.
#[tokio::test]
#[ignore = "requires an installed Go service and its authorized OS user"]
async fn installed_service_roundtrip() {
    let (_, bytes) = request("POST", "/rpc", br#"{"action":"status"}"#.to_vec(), 4 << 20)
        .await
        .expect("production local transport");
    let status: Value = serde_json::from_slice(&bytes).unwrap();
    assert_eq!(status["version"], env!("CARGO_PKG_VERSION"));
    assert_eq!(status["protocol_version"], 1);
    for field in ["private_key", "token", "certificate"] {
        assert!(status.get(field).is_none());
    }
    let (_, bytes) = request("POST", "/rpc", br#"{"action":"doctor"}"#.to_vec(), 4 << 20)
        .await
        .unwrap();
    let doctor: Value = serde_json::from_slice(&bytes).unwrap();
    assert!(doctor.get("platform").is_some());
    let rejected = request(
        "POST",
        "/rpc",
        br#"{"action":"service-install"}"#.to_vec(),
        4096,
    )
    .await
    .unwrap_err();
    assert_eq!(rejected.code, "unknown_action");
}

#[test]
fn refuses_admin_and_path_injection() {
    assert!(serde_json::from_str::<PlayerRequest>(r#"{"action":"service-install"}"#).is_err());
    assert!(
        serde_json::from_str::<PlayerRequest>(r#"{"action":"status","path":"/admin"}"#).is_err()
    );
    for id in ["../a", "a/b", "a?b", "a%2fb", ""] {
        assert!(!valid_id(id));
    }
    assert!(valid_id("steam-105600"));
}

#[tokio::test]
async fn http_body_limit_and_structured_failure() {
    use tokio::io::{AsyncReadExt, AsyncWriteExt};
    for (response, limit, expected) in [
            ("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\n12345", 4, "invalid_response"),
            ("HTTP/1.1 400 Bad Request\r\nContent-Length: 37\r\n\r\n{\"code\":\"forbidden\",\"error\":\"denied\"}", 100, "forbidden"),
        ] {
            let (client, mut server) = tokio::io::duplex(4096);
            tokio::spawn(async move {
                let mut buf = [0u8; 1024];
                let _ = server.read(&mut buf).await;
                server.write_all(response.as_bytes()).await.unwrap();
            });
            let err = exchange(client, "POST", "/rpc", vec![], limit).await.unwrap_err();
            assert_eq!(err.code, expected);
        }
}
