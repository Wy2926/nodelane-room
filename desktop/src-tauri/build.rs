fn main() {
    tauri_build::try_build(tauri_build::Attributes::new().app_manifest(
        tauri_build::AppManifest::new().commands(&[
            "player_request",
            "game_image",
            "exit_app",
            "notify_state",
        ]),
    ))
    .expect("desktop build configuration is invalid");
}
