use super::transport::exchange;
use super::*;

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
