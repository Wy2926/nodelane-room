#!/usr/bin/env bash
# Native installation only. Invoke after the complete function is downloaded.
nlroom_install() (
  set -euo pipefail
  umask 077
  local server='' version='' network='10.203.0.0/16' arg
  while (($#)); do
    arg=$1; shift
    case "$arg" in
      --server) server=${1:?missing server}; shift ;;
      --version) version=${1:?missing version}; shift ;;
      --network) network=${1:?missing network}; shift ;;
      *) echo "未知参数：$arg" >&2; return 1 ;;
    esac
  done
  [[ $(id -u) == 0 ]] || { echo '请在宿主机 root 终端运行。' >&2; return 1; }
  if [[ -e /.dockerenv || -e /run/.containerenv ]] || systemd-detect-virt --container --quiet 2>/dev/null; then
    echo '容器内请使用官方节点镜像和 Compose YAML；本脚本只安装宿主机 systemd 服务。' >&2; return 1
  fi
  [[ -d /run/systemd/system ]] || { echo '需要使用 systemd 的 Debian/Ubuntu 主机。' >&2; return 1; }
  . /etc/os-release
  case "$ID:$VERSION_ID" in debian:12|debian:13|ubuntu:22.04|ubuntu:24.04) ;; *) echo '支持 Debian 12/13、Ubuntu 22.04/24.04。' >&2; return 1 ;; esac
  local arch
  case "$(uname -m)" in x86_64) arch=amd64 ;; aarch64|arm64) arch=arm64 ;; *) echo '仅支持 amd64/arm64。' >&2; return 1 ;; esac
  local dependency
  for dependency in curl python3 tar sha256sum install systemctl runuser ip flock; do
    command -v "$dependency" >/dev/null || { echo "缺少 $dependency，请安装后重试。" >&2; return 1; }
  done
  [[ -c /dev/net/tun ]] || { echo '缺少 /dev/net/tun，请由宿主机管理员启用 TUN。' >&2; return 1; }
  [[ -r /dev/tty && -w /dev/tty ]] || { echo '需要交互终端输入临时接入密钥。' >&2; return 1; }
  server=${server%/}
  python3 - "$server" <<'PY'
import sys, urllib.parse
u=urllib.parse.urlsplit(sys.argv[1])
if u.scheme!='https' or not u.hostname or u.username or u.password or u.path or u.query or u.fragment or any(c.isspace() for c in sys.argv[1]):
    raise SystemExit('请通过 --server 指定完整的控制端 HTTPS origin。')
PY
  python3 - "$network" <<'PY'
import ipaddress,json,subprocess,sys
n=ipaddress.ip_network(sys.argv[1])
for link in json.loads(subprocess.check_output(['ip','-j','address'])):
    if link['ifname'] in ('lo','nodelane0'): continue
    for a in link.get('addr_info',[]):
        if a['family']=='inet' and n.overlaps(ipaddress.ip_network(f"{a['local']}/{a['prefixlen']}",strict=False)):
            raise SystemExit('组网地址池与接口 '+link['ifname']+' 冲突；请先处理网络重叠。')
for route in json.loads(subprocess.check_output(['ip','-j','route','show','table','all'])):
    if route.get('dst','default')=='default' or route.get('dev') in ('lo','nodelane0'): continue
    if n.overlaps(ipaddress.ip_network(route['dst'],strict=False)):
        raise SystemExit('组网地址池与本机路由 '+route['dst']+' 冲突；请先处理网络重叠。')
PY
  local path
  for path in /etc/nlroom-node /var/lib/nlroom-node /usr/local/bin/nlroom-node /etc/systemd/system/nlroom-node.service; do
    [[ ! -L "$path" ]] || { echo "拒绝符号链接路径：$path" >&2; return 1; }
  done
  install -d -m 0750 -o root -g root /etc/nlroom-node
  exec 9>/etc/nlroom-node/install.lock
  flock -n 9 || { echo '已有安装任务运行中。' >&2; return 1; }
  if [[ -f /etc/nlroom-node/config.json ]]; then
    python3 - "$server" <<'PY'
import json,sys
c=json.load(open('/etc/nlroom-node/config.json'))
if c.get('server')!=sys.argv[1]: raise SystemExit('已有安装属于另一个控制端；已保留原配置和身份。')
PY
    chown root:nlroom-node /etc/nlroom-node
    if [[ -x /usr/local/bin/nlroom-node && -f /etc/systemd/system/nlroom-node.service ]]; then
      systemctl enable --now nlroom-node.service
      /usr/local/bin/nlroom-node enroll
      return
    fi
  elif [[ -e /usr/local/bin/nlroom-node || -e /var/lib/nlroom-node/identity.bin ]]; then
    echo '检测到没有对应安装配置的程序或身份；请检查现有安装，脚本不会覆盖。' >&2; return 1
  fi
  local work
  work=$(mktemp -d /etc/nlroom-node/.install-XXXXXXXX)
  trap 'rm -rf -- "$work"' EXIT
  curl --proto '=https' --tlsv1.2 --fail --silent --show-error --connect-timeout 15 --max-time 60 "$server/install/manifest.json" -o "$work/manifest.json"
  python3 - "$work/manifest.json" "$arch" "$version" > "$work/artifact" <<'PY'
import json,re,sys
m=json.load(open(sys.argv[1]));v=m['version'];a=m['artifacts']['linux/'+sys.argv[2]]
if not re.fullmatch(r'0\.\d+\.\d+',v) or (sys.argv[3] and v!=sys.argv[3]): raise SystemExit('控制端未提供指定版本。')
if a['file']!=f'nlroom-node-{v}-linux-{sys.argv[2]}.tar.gz' or not re.fullmatch('[0-9a-f]{64}',a['sha256']): raise SystemExit('发布清单无效。')
print(v);print(a['file']);print(a['sha256'])
PY
  local file checksum
  { read -r version; read -r file; read -r checksum; } < "$work/artifact"
  curl --proto '=https' --tlsv1.2 --fail --silent --show-error --connect-timeout 15 --max-time 300 "$server/install/$file" -o "$work/release.tar.gz"
  [[ "$(sha256sum "$work/release.tar.gz" | cut -d ' ' -f 1)" == "$checksum" ]] || { echo '发布包校验失败。' >&2; return 1; }
  python3 - "$work" <<'PY'
import pathlib,tarfile,sys
p=pathlib.Path(sys.argv[1])
with tarfile.open(p/'release.tar.gz') as t:
    for name in ('nlroom-node','nlroom-node.service'):
        entries=[e for e in t.getmembers() if e.name==name]
        if len(entries)!=1 or not entries[0].isfile() or entries[0].size>128*1024*1024: raise SystemExit('发布包格式错误。')
        (p/name).write_bytes(t.extractfile(entries[0]).read())
PY
  if ! id nlroom-node >/dev/null 2>&1; then
    useradd --system --home-dir /var/lib/nlroom-node --shell /usr/sbin/nologin nlroom-node
  fi
  install -d -m 0700 -o nlroom-node -g nlroom-node /var/lib/nlroom-node
  runuser -u nlroom-node -- test -r /dev/net/tun && runuser -u nlroom-node -- test -w /dev/net/tun || {
    echo '服务账号不能读写 /dev/net/tun；请配置该设备的专用组或 udev 规则后重试。' >&2; return 1
  }
  chmod 0755 "$work/nlroom-node"
  "$work/nlroom-node" version
  python3 - "$server" "$version" "$work/config.json" <<'PY'
import json,sys
with open(sys.argv[3],'w') as f: json.dump({'server':sys.argv[1],'version':sys.argv[2]},f)
PY
  chown root:nlroom-node /etc/nlroom-node
  chmod 0750 /etc/nlroom-node
  install -m 0640 -o root -g nlroom-node "$work/config.json" /etc/nlroom-node/config.json
  # Save the installation ownership before replacing files so an interrupted
  # first installation can be safely retried against this same control server.
  install -m 0755 -o root -g root "$work/nlroom-node" /usr/local/bin/.nlroom-node-new
  mv -f /usr/local/bin/.nlroom-node-new /usr/local/bin/nlroom-node
  install -m 0644 -o root -g root "$work/nlroom-node.service" /etc/systemd/system/nlroom-node.service
  systemctl daemon-reload
  systemctl enable --now nlroom-node.service
  local attempt
  for attempt in {1..30}; do
    if /usr/local/bin/nlroom-node status --json >/dev/null 2>&1; then break; fi
    sleep 1
  done
  /usr/local/bin/nlroom-node enroll
  echo '安装完成。使用 nlroom-node status / doctor / logs 查看状态。'
)
nlroom_install "$@"
