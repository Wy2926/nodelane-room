(() => {
  const root = document.querySelector('[data-invitation]');
  if (!root) return;
  const en = document.documentElement.lang === 'en';
  const text = (zh, english) => en ? english : zh;
  const find = (name) => root.querySelector(`[data-invite-${name}]`);
  document.querySelector('.skip')?.addEventListener('click', (event) => {
    event.preventDefault(); // Preserve the invitation fragment when skipping navigation.
    const content = document.querySelector('#content');
    if (content) { content.tabIndex = -1; content.focus(); content.scrollIntoView(); }
  });
  let generation = 0, timer, request;
  async function refresh() {
    const current = ++generation;
    clearTimeout(timer);
    request?.abort();
    const controller = new AbortController();
    request = controller;
    const timeout = setTimeout(() => controller.abort(), 15000);
    const code = location.hash.slice(1);
    root.setAttribute('aria-busy', 'true');
    find('open').hidden = true;
    find('open').removeAttribute('href');
    find('room').hidden = true;
    find('status').textContent = text('正在读取房间信息…', 'Loading room information…');
    document.querySelector('.language-switch')?.setAttribute('href', `${en ? '/join' : '/en/join'}${location.hash}`);
    try {
      if (!/^[a-f0-9]{32}$/.test(code)) throw new Error('invite_unusable');
      const response = await fetch('/v2/invitations/preview', {
        method: 'POST', credentials: 'omit', cache: 'no-store', referrerPolicy: 'no-referrer',
        headers: {'Content-Type': 'application/json', 'X-NodeLane-Contract': 'interaction-1'},
        body: JSON.stringify({code}), signal: request.signal,
      });
      const envelope = await response.json();
      if (!response.ok || envelope.code !== 'ok') throw new Error(envelope.code);
      if (current !== generation) return;
      const room = envelope.data;
      find('name').textContent = room.name;
      find('game').textContent = room.game_name;
      find('members').textContent = text(`房间人数 ${room.member_count} / ${room.capacity}`, `Members ${room.member_count} / ${room.capacity}`);
      find('expiry').textContent = text('房间到期：', 'Room expires: ') + new Date(room.room_expires_at).toLocaleString(en ? 'en-US' : 'zh-CN');
      find('room').hidden = false;
      find('open').href = `nodelane-room://join#${code}`;
      find('open').hidden = false;
      find('status').textContent = text('邀请有效至 ', 'Invitation valid until ') + new Date(room.invite_expires_at).toLocaleString(en ? 'en-US' : 'zh-CN');
      // Use the server clock. Never turn local clock skew into a request loop.
      const remaining = Date.parse(room.invite_expires_at) - Date.parse(envelope.server_time);
      timer = setTimeout(() => {
        find('open').hidden = true;
        find('open').removeAttribute('href');
        find('status').textContent = text('请刷新以确认邀请是否仍然有效。', 'Refresh to check whether this invitation is still valid.');
      }, Number.isFinite(remaining) ? Math.max(1000, remaining) : 30000);
    } catch (error) {
      if (current !== generation) return;
      find('status').textContent = error.message === 'invite_unusable'
        ? text('邀请已失效或房间已关闭，请向房主要新的邀请链接。', 'This invitation is unavailable. Ask the owner for a new link.')
        : text('暂时无法读取房间，请刷新重试。', 'Cannot load the room right now. Please try again.');
    } finally {
      clearTimeout(timeout);
      if (current === generation) root.setAttribute('aria-busy', 'false');
    }
  }
  find('retry').addEventListener('click', refresh);
  find('open').addEventListener('click', () => {
    find('help').textContent = text('请在浏览器提示中允许打开客户端。如果没有响应，请先下载安装，再点击“打开客户端”。', 'Allow your browser to open the app. If nothing happens, install the app and click “Open the app” again.');
  });
  window.addEventListener('hashchange', refresh);
  window.addEventListener('focus', refresh);
  window.addEventListener('pageshow', refresh);
  window.addEventListener('pagehide', () => { ++generation; clearTimeout(timer); request?.abort(); });
  void refresh();
})();
