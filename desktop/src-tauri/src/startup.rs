use crate::language::Language;
use std::{
    os::windows::process::CommandExt,
    path::{Path, PathBuf},
    process::Command,
};

fn installed_helper(executable: &Path, programs: &Path) -> Option<PathBuf> {
    let directory = programs.join("NodeLaneRoom");
    if executable
        .as_os_str()
        .to_string_lossy()
        .eq_ignore_ascii_case(&directory.join("nlroom.exe").as_os_str().to_string_lossy())
    {
        Some(directory.join("nlroom-update.exe"))
    } else {
        None // Source builds do not provision a host network adapter.
    }
}

pub fn prepare_tap(language: Language) {
    use windows_sys::Win32::UI::{
        Shell::{FOLDERID_ProgramFiles, SHGetKnownFolderPath},
        WindowsAndMessaging::{MessageBoxW, MB_ICONWARNING, MB_OK},
    };
    let Ok(executable) = std::env::current_exe() else {
        return;
    };
    let mut folder = std::ptr::null_mut();
    if unsafe { SHGetKnownFolderPath(&FOLDERID_ProgramFiles, 0, std::ptr::null_mut(), &mut folder) }
        != 0
    {
        return;
    }
    let mut length = 0;
    unsafe {
        while *folder.add(length) != 0 {
            length += 1;
        }
    }
    let programs = PathBuf::from(unsafe {
        String::from_utf16_lossy(std::slice::from_raw_parts(folder, length))
    });
    unsafe {
        windows_sys::Win32::System::Com::CoTaskMemFree(folder.cast());
    }
    let Some(helper) = installed_helper(&executable, &programs) else {
        return;
    };
    // The helper checks first and requests UAC only when TAP is missing. Keep
    // the ordinary GUI available for settings/diagnostics if UAC is cancelled.
    let result = Command::new(helper)
        .arg("tap")
        .creation_flags(0x08000000)
        .output();
    if matches!(result, Ok(ref output) if output.status.success()) {
        return;
    }
    let detail = match result {
        Ok(output) => format!(
            "{}\n{}",
            String::from_utf8_lossy(&output.stdout).trim(),
            String::from_utf8_lossy(&output.stderr).trim()
        ),
        Err(error) => error.to_string(),
    };
    let detail: String = detail.chars().take(1024).collect();
    let message: Vec<u16> = format!("{}\n\n{}", language.text("native.tapFailed"), detail.trim())
        .encode_utf16()
        .chain(Some(0))
        .collect();
    let title: Vec<u16> = "NodeLane Room".encode_utf16().chain(Some(0)).collect();
    unsafe {
        MessageBoxW(
            std::ptr::null_mut(),
            message.as_ptr(),
            title.as_ptr(),
            MB_OK | MB_ICONWARNING,
        );
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn only_the_installed_gui_prepares_tap() {
        let programs = Path::new(r"C:\Program Files");
        assert_eq!(
            installed_helper(
                Path::new(r"C:\Program Files\NodeLaneRoom\nlroom.exe"),
                programs
            ),
            Some(programs.join("NodeLaneRoom/nlroom-update.exe"))
        );
        assert!(installed_helper(Path::new(r"D:\build\nlroom.exe"), programs).is_none());
        assert!(
            installed_helper(Path::new(r"C:\Program Files\Other\nlroom.exe"), programs).is_none()
        );
    }
}
