use serde::{Deserialize, Serialize};
use serde_json::Value;
#[derive(Debug, Serialize, Deserialize)]
pub struct Failure {
    pub code: String,
    #[serde(rename = "error")]
    pub message: String,
}

impl Failure {
    pub fn new(code: &str, message: &str) -> Self {
        Self {
            code: code.into(),
            message: message.into(),
        }
    }
}

#[derive(Deserialize, Serialize, Debug)]
#[serde(rename_all = "kebab-case")]
pub enum Action {
    Status,
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
