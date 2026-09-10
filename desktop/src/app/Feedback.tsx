import type { useService } from "../native/use-service";
import type { Actions } from "./use-actions";
import { formatTime } from "../shared/time";

export function Feedback({
  service,
  actions,
  refreshAll,
}: {
  service: ReturnType<typeof useService>;
  actions: Actions;
  refreshAll: () => void;
}) {
  const { status, error: serviceError, refresh } = service;
  const { dialog, error, notice, busy } = actions;
  return (
    <>
      {serviceError && (
        <div className="banner error" role="alert">
          <div>
            <strong>无法连接网络后台</strong>
            <p>{serviceError.error}</p>
          </div>
          <div className="actions"><button onClick={refresh}>重新检查</button><button onClick={() => void actions.quit()}>退出界面</button></div>
        </div>
      )}
      {status?.error && (
        <div className="banner warning" role="status">
          <div>
            <strong>
              {status.control === "unreachable"
                ? "控制服务暂时不可达"
                : "网络需要关注"}
            </strong>
            <p>{status.error}</p>
            {status.lease_expires_at && (
              <small>当前授权截止：{formatTime(status.lease_expires_at)}</small>
            )}
          </div>
          <button onClick={refreshAll}>刷新状态</button>
        </div>
      )}
      {!dialog && error && (
        <div className="banner error" role="alert">
          {error.error}
        </div>
      )}
      {notice && (
        <div className="toast" role="status">
          {notice}
        </div>
      )}
      {busy && (
        <div className="working" role="status">
          {busy}…
        </div>
      )}
    </>
  );
}
