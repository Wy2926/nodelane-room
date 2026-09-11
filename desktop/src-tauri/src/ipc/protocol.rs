use serde::{Deserialize, Serialize};
use serde_json::Value;
#[derive(Debug, Serialize, Deserialize)]
pub struct Failure {
    pub code: String,
    pub message: String,
    #[serde(flatten)]
    pub metadata: serde_json::Map<String, Value>,
}

impl Failure {
    pub fn new(code: &str, message: &str) -> Self {
        use std::sync::atomic::{AtomicU64, Ordering};
        static SEQUENCE: AtomicU64 = AtomicU64::new(0);
        let stamp = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap_or_default()
            .as_nanos();
        let id = format!(
            "{:x}-{:x}-{:x}",
            std::process::id(),
            stamp,
            SEQUENCE.fetch_add(1, Ordering::Relaxed)
        );
        Self {
            code: code.into(),
            message: message.into(),
            metadata: serde_json::from_value(serde_json::json!({"contract":"interaction-1","request_id":id,"origin":"bridge","control_http_status":null,"data":null,"details":{},"retry":{"kind":"manual"}})).unwrap_or_default(),
        }
    }
}

#[derive(Deserialize, Serialize, Debug)]
#[serde(rename_all = "kebab-case")]
pub enum Action {
    Capabilities,
    GetOperation,
    NetworkStop,
    NetworkRetry,
    InviteInfo,
    InviteRevoke,
    OwnerJoin,
    AccountDevices,
    AccountStatus,
    RevokeDevice,
    Status,
    UpdateStatus,
    UpdateCheck,
    UpdateInstall,
    AccountLogin,
    AccountLink,
    AccountPoll,
    AccountCancel,
    AccountLogout,
    AccountTakeover,
    Init,
    Games,
    Rooms,
    Manage,
    Members,
    Create,
    Join,
    Invite,
    Kick,
    Transfer,
    Leave,
    Close,
    Ping,
    Doctor,
}

#[derive(Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct PlayerRequest {
    pub contract: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub command_id: String,
    pub action: Action,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub room: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub server: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub name: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub target: String,
    #[serde(default)]
    pub body: Value,
}

pub(super) fn valid_id(id: &str) -> bool {
    !id.is_empty()
        && id.len() <= 128
        && id
            .bytes()
            .all(|c| c.is_ascii_alphanumeric() || c == b'-' || c == b'_')
}
