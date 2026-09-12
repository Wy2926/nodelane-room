use std::sync::Mutex;
use tauri::{Emitter, Manager};

#[derive(Default)]
pub struct PendingInvitation(Mutex<Option<String>>);

fn code(argument: &str) -> Option<&str> {
    let code = argument.strip_prefix("nodelane-room://join#")?;
    (code.len() == 32
        && code
            .bytes()
            .all(|c| c.is_ascii_digit() || (b'a'..=b'f').contains(&c)))
    .then_some(code)
}

pub fn receive(app: &tauri::AppHandle, args: impl Iterator<Item = String>) {
    if let Some(value) = args.filter_map(|arg| code(&arg).map(str::to_owned)).last() {
        let pending = app.state::<PendingInvitation>();
        *pending.0.lock().unwrap_or_else(|err| err.into_inner()) = Some(value);
        // Event contains no invitation; only the bundled window can consume it.
        let _ = app.emit_to("main", "invitation-ready", ());
    }
}

#[tauri::command]
pub fn take_invitation(pending: tauri::State<'_, PendingInvitation>) -> Option<String> {
    pending
        .0
        .lock()
        .unwrap_or_else(|err| err.into_inner())
        .take()
}

#[cfg(test)]
mod tests {
    use super::code;
    #[test]
    fn accepts_only_bounded_room_invitation() {
        let valid = "nodelane-room://join#0123456789abcdef0123456789abcdef";
        assert!(code(valid).is_some());
        for bad in [
            "nodelane-room://join",
            "https://evil.test/join#0123456789abcdef0123456789abcdef",
            "nodelane-room://join#0123456789abcdef0123456789abcdef --server=x",
            "nodelane-room://join?server=evil#0123456789abcdef0123456789abcdef",
            "nodelane-room://join#../../etc/passwd",
        ] {
            assert!(code(bad).is_none());
        }
    }
}
