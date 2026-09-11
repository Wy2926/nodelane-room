"""Disposable integration-test administrator. Credentials enter through stdin only."""
import http.cookiejar
import json
import ssl
import sys
import urllib.request
import uuid
import time
import datetime

def main():
    config = json.load(sys.stdin)
    server = config.get('server', 'https://control:8443')
    jar = http.cookiejar.CookieJar()
    context = ssl.create_default_context(cafile=config.get('ca', '/trust/control.crt'))
    opener = urllib.request.build_opener(urllib.request.HTTPSHandler(context=context), urllib.request.HTTPCookieProcessor(jar))
    csrf = ''
    def call(path, body=None, method='POST'):
        headers = {'X-NodeLane-Contract':'interaction-1','X-NodeLane-Operation-Deadline':(datetime.datetime.now(datetime.timezone.utc)+datetime.timedelta(minutes=50)).isoformat().replace('+00:00','Z'),'Content-Type':'application/json','Origin':server,'X-CSRF-Token':csrf,'Idempotency-Key':uuid.uuid4().hex}
        request = urllib.request.Request(server+'/v2/admin/'+path, data=json.dumps(body).encode() if body is not None else None, headers=headers, method=method)
        with opener.open(request, timeout=15) as response:
            result=json.load(response)
            if result.get("contract")!="interaction-1" or not result.get("request_id"): raise RuntimeError("invalid control contract")
            return result["data"]
    if config.get('code'):
        call('setup', {**{k:config[k] for k in ('code','username','password','database_url')}, 'mode':'create','public_url':server,'network':'10.203.0.0/16','registry':'docker.nodelane.net','ca_mode':'generate'})
        for attempt in range(30):
            if call('setup', method='GET')['initialized']: break
            time.sleep(1)
        else: raise RuntimeError('control did not become ready after setup')
    csrf = call('login', {k:config[k] for k in ('username','password')})['csrf']
    if config.get('create'):
        node = call('nodes',config['create'])
        result = {'node':node, **call('nodes/'+node['id']+'/key',{})}
    else:
        result = call(config['path'],config.get('body'),config.get('method','POST'))
    if config.get('wait_telemetry'):
        deadline = time.monotonic() + 90
        while time.monotonic() < deadline:
            ready = True
            for device in config['wait_telemetry']:
                series = next((s for s in result['series'] if s['device_id'] == device), None)
                samples = series['samples'] if series else []
                stamp = lambda s: datetime.datetime.fromisoformat(s['at'].replace('Z', '+00:00'))
                if len(samples) < 7 or (stamp(samples[-1]) - stamp(samples[0])).total_seconds() < 30:
                    ready = False
            if ready:
                break
            time.sleep(5)
            result = call('telemetry', method='GET')
        else:
            raise RuntimeError('telemetry window did not reach 30 seconds')
    print(json.dumps(result))

if __name__ == '__main__': main()
