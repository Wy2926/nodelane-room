"""Drive the installed Linux WebView against a disposable, real Go service."""
import base64
import http.client
import json
import os
from pathlib import Path
import subprocess
import sys
import time
import urllib.error
import urllib.request


def wait(check, label, seconds=60):
    end = time.monotonic() + seconds
    while time.monotonic() < end:
        try:
            value = check()
            if value: return value
        except (urllib.error.URLError, http.client.HTTPException, KeyError, RuntimeError): pass
        time.sleep(0.3)
    raise RuntimeError('Timed out: ' + label)


def main():
    # Invitation arrives on stdin, never in a process argument or result log.
    invitation = json.load(sys.stdin)['invitation']
    driver = subprocess.Popen(['tauri-driver', '--port', '4444'], stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    session = ''
    exited = False

    def call(method, path, body=None):
        request = urllib.request.Request('http://127.0.0.1:4444' + path, method=method,
            data=json.dumps(body).encode() if body is not None else None,
            headers={'Content-Type': 'application/json'})
        try:
            with urllib.request.urlopen(request, timeout=60) as response: result = json.load(response)
        except urllib.error.HTTPError as exc:
            # Driver errors may quote element text, including an invitation.
            code = json.load(exc).get('value', {}).get('error', 'unknown')
            raise RuntimeError('WebDriver command failed: ' + str(code) + ' ' + method + ' ' + path) from None
        return result.get('value')

    def script(code, *args):
        return call('POST', '/session/' + session + '/execute/sync', {'script': code, 'args': list(args)})

    def element(css):
        return call('POST', '/session/' + session + '/element', {'using': 'css selector', 'value': css})['element-6066-11e4-a52e-4f735466cecf']

    def click(css):
        item = wait(lambda: element(css), 'visible control')
        script('arguments[0].scrollIntoView({block:"center"})', {'element-6066-11e4-a52e-4f735466cecf': item})
        call('POST', f'/session/{session}/element/{item}/click', {})

    def button(label):
        item = wait(lambda: script("return [...document.querySelectorAll('button')].find(e => e.textContent.trim() === arguments[0] && !e.disabled && e.getClientRects().length)", label), 'enabled button')
        script('arguments[0].scrollIntoView({block:"center"})', item)
        call('POST', f'/session/{session}/element/{item["element-6066-11e4-a52e-4f735466cecf"]}/click', {})

    def fill(css, text):
        item = wait(lambda: element(css), 'input')
        call('POST', f'/session/{session}/element/{item}/clear', {})
        call('POST', f'/session/{session}/element/{item}/value', {'text': text})

    def contains(text):
        return script('return document.body.textContent.includes(arguments[0])', text)

    def status():
        return json.loads(subprocess.check_output(['nlroom-cli', 'status', '--json'], stderr=subprocess.DEVNULL))

    try:
        session = wait(lambda: call('POST', '/session', {'capabilities': {'alwaysMatch': {'tauri:options': {'application': '/usr/bin/nlroom'}}}}), 'native WebView startup')['sessionId']
        fill('input[name=name]', '桌面验收玩家')
        click('.server-choice summary')
        fill('input[name=server]', 'https://control:8443')
        button('开始旅程')
        wait(lambda: status().get('device_id'), 'real identity initialization')
        button('游戏库')
        click('#game-custom')
        button('创建房间')
        fill('dialog input[name=name]', '原生联机验收')
        button('创建并连接')
        first = wait(lambda: script("return document.querySelector('.invitation')?.textContent"), 'created invitation')
        click('dialog button[aria-label="关闭对话框"]')
        wait(lambda: status()['engine'] == 'running', 'real Nebula network')
        fill('input[name=port]', '26001')
        button('添加')
        wait(lambda: contains('已登记'), 'authorized game port')
        button('删除')
        wait(lambda: not script("return !!document.querySelector('.port-row')"), 'port removal')
        button('生成新邀请码')
        renewed = wait(lambda: script("return document.querySelector('.invitation')?.textContent"), 'renewed invitation')
        if first == renewed: raise RuntimeError('Invitation was not rotated')
        button('复制邀请码')
        wait(lambda: contains('已复制'), 'native clipboard')
        click('dialog button[aria-label="关闭对话框"]')
        button('离开房间')
        button('确认离开房间')
        wait(lambda: not status().get('selected_room'), 'leave acknowledgement')
        click('.room-tabs button')
        wait(lambda: contains('仅管理'), 'owner management after leave')
        button('生成新邀请码')
        wait(lambda: script("return !!document.querySelector('.invitation')"), 'management invitation')
        click('dialog button[aria-label="关闭对话框"]')
        button('关闭房间')
        button('确认关闭房间')
        click('.welcome-cards button:last-child')
        fill('dialog input[name=code]', invitation)
        button('加入并连接')
        wait(lambda: status()['engine'] == 'running' and len(status()['members']) == 2, 'join and real network')
        button('测延迟')
        wait(lambda: contains('最近探测'), 'real peer probe')
        Path('/output/room.png').write_bytes(base64.b64decode(call('GET', '/session/' + session + '/screenshot')))
        button('网络诊断')
        button('运行诊断')
        wait(lambda: script("return !!document.querySelector('.system-checks') && !document.querySelector('pre')"), 'visual diagnostic response')
        button('复制脱敏诊断')
        button('设置')
        button('退出与联机')
        try: button('退出界面（继续联机）')
        except (RuntimeError, urllib.error.URLError, http.client.HTTPException): pass
        wait(lambda: subprocess.run(['pgrep', '-u', str(os.getuid()), '-x', 'nlroom'], stdout=subprocess.DEVNULL).returncode == 1, 'GUI process exit', 10)
        exited = True
        wait(lambda: status()['engine'] == 'running', 'service survives GUI exit')
        print('PASS native WebView: init, catalog, create, ports, invitation rotation/copy, leave/manage/close, join, measured ping, diagnostics and GUI exit')
    except Exception:
        if session:
            try:
                script("document.querySelectorAll('.invitation,input[name=code]').forEach(e => e.style.visibility='hidden')")
                Path('/output/failure.png').write_bytes(base64.b64decode(call('GET', '/session/' + session + '/screenshot')))
            except Exception: pass
        raise
    finally:
        if session and not exited:
            try: call('DELETE', '/session/' + session)
            except (RuntimeError, urllib.error.URLError, http.client.HTTPException): pass
        driver.terminate()
        startup_error, _ = driver.communicate(timeout=10)
        if not session and startup_error:
            sys.stderr.write(startup_error.decode('utf-8', errors='replace')[-4000:])


if __name__ == '__main__': main()
