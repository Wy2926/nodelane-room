use serde::Deserialize;
use std::collections::HashMap;
use std::sync::LazyLock;

#[derive(Clone, Copy, Default, Deserialize)]
pub enum Language {
    #[default]
    #[serde(rename = "zh-CN")]
    Chinese,
    #[serde(rename = "en-US")]
    English,
}

static CHINESE: LazyLock<HashMap<String, String>> = LazyLock::new(|| {
    serde_json::from_str(include_str!("../../src/i18n/locales/zh-CN.json"))
        .expect("invalid bundled Chinese dictionary")
});
static ENGLISH: LazyLock<HashMap<String, String>> = LazyLock::new(|| {
    serde_json::from_str(include_str!("../../src/i18n/locales/en-US.json"))
        .expect("invalid bundled English dictionary")
});

impl Language {
    pub fn text(self, key: &str) -> &'static str {
        let dictionary = match self {
            Self::Chinese => &*CHINESE,
            Self::English => &*ENGLISH,
        };
        dictionary.get(key).expect("missing bundled native message")
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn native_messages_are_available_in_both_languages() {
        for key in [
            "native.open",
            "native.leave",
            "native.quit",
            "native.disconnected",
            "native.disabled",
            "native.controlUnavailable",
        ] {
            assert_ne!(Language::Chinese.text(key), Language::English.text(key));
        }
        assert!(serde_json::from_str::<Language>("\"fr-FR\"").is_err());
    }
}
