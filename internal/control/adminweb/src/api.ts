import type { API } from "./types";

const messages: Record<string, string> = {
  ok: "按操作显示已加入/已保存等",
  operation_noop: "已处于目标状态",
  operation_pending: "正在处理，可查看进度",
  operation_result_redacted: "操作已确认，当前状态已变化",
  operation_not_found: "暂未查询到结果",
  operation_expired: "原操作结果已无法继续查询，请核对当前状态",
  request_malformed: "请求格式不受支持，请更新客户端",
  request_validation_failed: "请检查标出的内容",
  request_too_large: "内容超过允许大小",
  request_media_unsupported: "文件或请求类型不受支持",
  request_method_unsupported: "客户端请求方式不受支持",
  request_idempotency_required: "请求标识无效，请刷新后重试",
  request_idempotency_conflict: "操作内容与原请求不一致",
  request_state_stale: "状态已变化，请核对后重新确认",
  request_rate_limited: "操作较频繁，请在倒计时后重试",
  request_origin_rejected: "请从正确的服务页面重新打开",
  request_csrf_rejected: "页面验证已失效，请刷新",
  api_contract_unsupported: "客户端版本不兼容，请更新",
  resource_not_found: "无法访问该资源",
  system_not_ready: "联机服务尚未就绪",
  system_unavailable: "联机服务暂不可用",
  system_upstream_timeout: "外部服务响应超时",
  system_ca_unavailable: "联机授权暂不可签发，请联系管理员",
  system_internal_error: "服务发生异常，可凭追踪编号反馈",
  auth_session_required: "正在恢复登录",
  auth_challenge_unusable: "登录验证已失效，正在重试",
  auth_proof_invalid: "无法验证此设备，请重新发起登录",
  auth_device_unregistered: "此设备尚未登录",
  auth_device_expired: "此设备授权已到期，请重新登录",
  auth_device_revoked: "此设备已被退出登录",
  auth_identity_scope_conflict: "设备身份用途不匹配",
  account_disabled: "账号已停用，请联系管理员",
  account_deleted: "此账号已不可使用",
  account_in_use: "账号正在另一设备联机",
  account_device_limit: "已达设备数量上限，请先退出旧设备",
  account_identity_conflict: "该身份已有账号，访客数据不会合并",
  account_link_not_allowed: "当前账号不适用访客绑定",
  account_guest_logout_forbidden: "请先绑定账号，以免丢失访客身份",
  auth_oidc_unavailable: "当前服务未开放账号登录",
  auth_login_pending: "请在浏览器完成登录",
  auth_login_confirmation_required: "请在浏览器确认这台设备",
  auth_login_denied: "你已取消此次登录",
  auth_login_expired: "登录已超时，请重新开始",
  auth_login_unusable: "此次登录已不能继续，请核对登录状态",
  auth_login_config_changed: "登录配置已变化，请重新开始",
  auth_login_validation_failed: "登录验证未通过，请重新开始",
  auth_login_provider_unavailable: "账号服务暂不可用",
  auth_login_cancelled: "已停止等待；浏览器已确认的授权仍可能有效",
  room_already_joined: "请先离开当前房间",
  room_full: "房间已满，请稍后再试",
  room_banned: "你已被禁止加入此房间",
  room_closed: "房间已解散",
  room_expired: "房间已到期",
  room_owner_required: "你已没有此房间的管理权限",
  game_not_found: "游戏已不在目录中，请重新选择",
  game_disabled: "此游戏已暂停联机",
  invite_unusable: "邀请码不可用，请向房主要新码",
  invite_expired: "邀请码已到期",
  invite_changed: "邀请状态已更新",
  member_not_active: "该成员已不在房间",
  member_target_self: "请选择其他成员；离开请使用离房",
  member_kicked: "你已被移出并禁止再次加入此房间",
  member_taken_over: "此账号已在另一设备接管联机",
  member_left: "已离开房间",
  member_revoked: "你的房间连接授权已结束",
  network_address_exhausted: "暂无可用联机地址，请稍后重试",
  lan_version_unsupported: "客户端不支持此房间所需LAN版本",
  lan_mac_conflict: "虚拟网卡地址冲突，请检查专用网卡",
  lan_mac_pending: "正在登记虚拟网卡",
  lan_policy_changed: "网络配置已更新，正在重新连接",
  lease_replay_obsolete: "正在重新获取联机授权",
  client_update_required: "请更新客户端后继续联机",
  local_desktop_required: "请在已安装的客户端中使用",
  local_service_unavailable: "本机服务不可用，请检查服务",
  local_permission_denied: "当前系统用户无法访问本机服务",
  local_protocol_incompatible: "客户端与服务不兼容，请修复安装",
  local_version_mismatch: "界面和服务版本不一致",
  local_unconfigured: "请先完成初次设置或登录",
  local_already_initialized: "此设备已完成设置",
  local_no_room: "当前没有连接房间",
  local_busy: "正在处理前一项操作",
  local_rpc_timeout: "本机响应超时，正在确认结果",
  local_request_cancelled: "已停止等待",
  local_network_offline: "当前网络已断开",
  local_dns_failed: "无法解析联机服务地址",
  local_tls_failed: "无法验证联机服务身份",
  local_control_unreachable: "无法连接联机服务",
  local_control_timeout: "请求超时，正在确认结果",
  local_control_connection_lost: "连接中断，操作结果待确认",
  local_control_response_invalid: "联机服务响应无法识别",
  local_ipc_response_invalid: "本机服务响应不兼容",
  local_storage_failed: "无法保存本机状态，请检查安装或存储",
  local_identity_unreadable: "无法读取设备身份，请修复安装或重新登录",
  local_clock_skew: "系统时间与服务不一致，请同步时间",
  local_ca_changed: "联机服务身份已变化，请联系管理员",
  local_lease_invalid: "联机凭据校验失败",
  local_lease_expired: "联机凭据已到期，连接已暂停",
  local_authorization_expired: "房间授权未能续期，游戏连接已暂停",
  local_policy_invalid: "收到的网络配置无效",
  local_tap_missing: "未找到所需虚拟网卡，请完成网卡准备",
  local_tap_invalid: "虚拟网卡不符合要求",
  local_tap_unavailable: "虚拟网卡暂不可用",
  local_platform_unsupported: "当前系统不支持此LAN网络功能",
  local_route_conflict: "联机网段与本机网络冲突",
  local_udp_bind_failed: "联机端口无法使用",
  local_engine_start_failed: "网络组件启动失败，请查看诊断",
  local_network_stop_failed: "无法确认网络已停止，请检查服务",
  local_lan_prepare_timeout: "LAN准备尚未完成，请检查网络或网卡",
  local_discovery_unavailable: "暂无可用发现节点，部分连接可能无法建立",
  local_peer_unreachable: "尚未探测到该成员，请检查双方网络",
  local_probe_unavailable: "暂时无法测量连接质量",
  local_probe_target_ambiguous: "昵称重复，请选择具体设备或虚拟IP",
  local_probe_target_unavailable: "该成员当前无法探测",
  local_stream_disconnected: "正在恢复房间状态同步",
  event_snapshot_reset: "通常无需打扰用户",
  local_state_stale: "状态尚未更新，正在重连",
  local_state_inconsistent: "正在重新核对房间状态",
  local_browser_open_failed: "无法打开浏览器，可再次打开登录页面",
  local_clipboard_failed: "复制未成功，可手动选择文本",
  local_preference_save_failed: "设置未保存",
  local_image_unavailable: "使用默认封面",
  local_internal_error: "本机发生异常，可复制诊断编号",
  local_update_check_failed: "暂时无法检查更新",
  local_update_trust_unconfigured: "当前安装无法自动更新，请使用完整安装包",
  local_update_metadata_invalid: "更新信息未通过验证",
  local_update_package_invalid: "更新包未通过校验",
  local_update_download_failed: "更新下载中断，可以重试",
  local_update_disk_full: "存储空间不足，请清理后重试",
  local_update_storage_failed: "无法保存更新状态",
  local_update_rollback_unavailable: "缺少可用恢复包，暂不能安装",
  local_update_rollback_invalid: "恢复包校验失败",
  local_update_release_changed: "更新版本已变化，正在重新检查",
  local_update_not_ready: "更新尚未准备完成",
  local_update_install_unsupported: "请使用完整安装包进行更新",
  local_update_elevation_cancelled: "已取消安装，可稍后再试",
  local_update_install_busy: "安装正在进行，请等待",
  local_update_install_failed: "安装失败，正在检查恢复状态",
  local_update_stop_failed: "服务尚未停止，更新未能继续",
  local_update_health_failed: "新版本未正常启动，正在恢复",
  local_update_rolled_back: "已恢复到上一版本",
  local_update_recovery_failed: "自动恢复失败，需要修复安装",
  local_update_downgrade_rejected: "不允许安装此旧版本",
  setup_code_unusable: "初始化授权不可用，请在此实例重新获取",
  setup_already_configured: "此实例已配置，请查看当前状态",
  setup_database_incompatible: "数据库不符合当前结构或部署要求",
  setup_ca_invalid: "CA材料不符合要求",
  setup_binding_failed: "实例配置未能完成，请核对同一数据库后重试",
  admin_credentials_invalid: "账号或密码不正确",
  admin_session_required: "管理登录已失效，请重新登录",
  admin_current_password_invalid: "当前密码不正确",
  admin_password_changed: "密码已修改，请重新登录",
  admin_user_deleted: "此账号已删除，无法执行此操作",
  game_policy_missing: "请先配置允许的游戏网络规则",
  game_import_unavailable: "游戏资料暂不可获取，请稍后重试",
  game_import_invalid: "此游戏资料无法导入，请检查链接或稍后重试",
  update_metadata_rejected: "签名更新元数据未通过校验",
  update_release_duplicate: "此平台版本已存在",
  update_release_immutable: "此发布内容或状态不可更改",
  update_release_referenced: "请先切换或移除引用此版本的策略",
  update_source_required: "请先验证至少一个可用更新源",
  update_last_source_required: "请先准备备用源或调整策略",
  update_targets_in_use: "请保留仍在使用的更新目标",
  update_policy_invalid: "更新策略与所选版本不匹配",
  update_source_unavailable: "无法访问更新源，请检查地址和权限",
  update_source_verification_failed: "更新源上的文件未通过校验",
  update_source_credentials_unavailable:
    "更新源配置暂不可用，请检查控制实例配置",
  node_session_required: "正在恢复节点认证",
  node_enrollment_unusable: "登记凭据不可用，请核对节点状态",
  node_not_authorized: "此机器没有有效节点授权",
  node_revoked: "节点已永久撤销",
  node_disabled: "节点已停用",
  node_generation_stale: "机器绑定已更新",
  node_state_conflict: "当前节点状态不允许此操作",
  node_applied_config_invalid: "节点应用状态无法验证",
  node_apply_required: "等待在节点上应用配置",
  node_offline: "节点离线，操作待执行",
  node_operation_expired: "操作已过期，未继续执行",
  node_operation_superseded: "此操作已由更新配置替代",
  node_operation_failed: "节点执行失败，请检查节点诊断",
  node_public_endpoint_unverified: "公网入口尚未验证",
};
export type PendingOperation = { id: string; path: string; deadline: string };
const ledgerKey = "nlroom.admin.operations.interaction-1";
export function pendingOperations(): PendingOperation[] {
  try {
    return JSON.parse(
      sessionStorage.getItem(ledgerKey) || "[]",
    ) as PendingOperation[];
  } catch {
    return [];
  }
}
export function forgetOperation(id: string) {
  sessionStorage.setItem(
    ledgerKey,
    JSON.stringify(pendingOperations().filter((op) => op.id !== id)),
  );
}
export const businessMessage = (code: string) =>
  messages[code] || "结果暂不支持，请凭请求编号查询。";
export class APIError extends Error {
  constructor(
    message: string,
    public status: number | null,
    public code = "system_internal_error",
    public request_id = "",
    public operation_id = "",
    public details: Record<string, unknown> = {},
    public retry: { kind: string; after_ms?: number } = { kind: "none" },
  ) {
    super(message);
  }
}
export function makeAPI(csrf: string, onUnauthorized: () => void): API {
  return async <T>(
    path: string,
    body?: unknown,
    method = body === undefined ? "GET" : "POST",
    signal?: AbortSignal,
  ): Promise<T> => {
    const headers: Record<string, string> = {
      "Content-Type":
        body instanceof Blob ? "application/octet-stream" : "application/json",
      "X-NodeLane-Contract": "interaction-1",
    };
    const id = crypto.randomUUID().replaceAll("-", "");
    const write = method !== "GET" && method !== "HEAD";
    const tracked =
      write &&
      !["/login", "/logout", "/password", "/setup"].includes(path) &&
      !path.endsWith("/key") &&
      !path.endsWith("/test") &&
      !/^\/updates\/releases\/[^/]+\/sources\//.test(path);
    const deadline = new Date(Date.now() + 3600000).toISOString();
    if (write) {
      headers["X-CSRF-Token"] = csrf;
      headers["Idempotency-Key"] = id;
      headers["X-NodeLane-Operation-Deadline"] = deadline;
    }
    if (tracked) {
      if (
        pendingOperations().some(
          (op) => op.path === path && Date.parse(op.deadline) > Date.now(),
        )
      )
        throw new APIError(
          "此操作的结果仍待确认，请先查询原操作。",
          null,
          "operation_pending",
        );
      try {
        sessionStorage.setItem(
          ledgerKey,
          JSON.stringify(
            [...pendingOperations(), { id, path, deadline }].slice(-32),
          ),
        );
      } catch {
        throw new APIError(
          "无法保存操作标识，请检查浏览器存储。",
          null,
          "local_storage_failed",
        );
      }
    }
    let res: Response;
    try {
      res = await fetch("/v2/admin" + path, {
        method,
        headers,
        body:
          body === undefined
            ? undefined
            : body instanceof Blob
              ? body
              : JSON.stringify(body),
        signal,
      });
    } catch {
      throw new APIError(
        "连接中断，操作结果待确认。",
        null,
        "local_control_connection_lost",
        "",
        tracked ? id : "",
      );
    }
    const data = await res.json().catch(() => {
      throw new APIError(
        "控制端响应无法读取，请查询原操作。",
        res.status,
        "local_control_response_invalid",
        "",
        tracked ? id : "",
      );
    });
    if (
      !data ||
      data.contract !== "interaction-1" ||
      !data.request_id ||
      !data.code
    )
      throw new APIError(
        "控制端响应不兼容。",
        res.status,
        "local_control_response_invalid",
        "",
        tracked ? id : "",
      );
    if (!res.ok) {
      if (data.code === "admin_session_required" && path !== "/login")
        onUnauthorized();
      if (
        tracked &&
        Object.hasOwn(messages, data.code) &&
        res.status < 500 &&
        res.status !== 429
      )
        forgetOperation(id);
      throw new APIError(
        messages[data.code] || "操作未完成，请凭请求编号查询。",
        res.status,
        data.code,
        data.request_id,
        data.operation_id || id,
        data.details,
        data.retry,
      );
    }
    if (
      ![
        "ok",
        "operation_noop",
        "operation_pending",
        "operation_result_redacted",
        "admin_password_changed",
      ].includes(data.code)
    )
      throw new APIError(
        "响应无法确认，请查询原操作。",
        res.status,
        "local_control_response_invalid",
        data.request_id,
        tracked ? id : "",
      );
    if (tracked) forgetOperation(id);
    return data.data as T;
  };
}

export function watchAdmin(
  onSnapshot: (value: unknown) => void,
  onError: () => void,
  signal: AbortSignal,
) {
  let timer: ReturnType<typeof setTimeout>;
  async function connect() {
    try {
      const response = await fetch("/v2/admin/events", {
        headers: { "X-NodeLane-Contract": "interaction-1" },
        signal,
      });
      if (!response.ok || !response.body) throw new Error();
      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";
      try {
        while (!signal.aborted) {
          const { value, done } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });
          if (buffer.length > 4194304) throw new Error();
          let split;
          while ((split = buffer.indexOf("\n\n")) >= 0) {
            const event = buffer.slice(0, split);
            buffer = buffer.slice(split + 2);
            const kind = event
              .split("\n")
              .find((line) => line.startsWith("event: "))
              ?.slice(7);
            const raw = event
              .split("\n")
              .find((line) => line.startsWith("data: "))
              ?.slice(6);
            if (kind === "terminal") throw new Error();
            if (kind === "snapshot" && raw) onSnapshot(JSON.parse(raw));
          }
        }
      } finally {
        await reader.cancel().catch(() => {});
      }
    } catch {
      if (!signal.aborted) onError();
    }
    if (!signal.aborted) timer = setTimeout(connect, 2000);
  }
  void connect();
  return {
    close() {
      clearTimeout(timer);
    },
  };
}
