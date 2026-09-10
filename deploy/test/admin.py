"""Disposable integration-test administrator. Credentials enter through stdin only."""
import http.cookiejar
import json
import ssl
import sys
import urllib.request
import uuid

def main():
    config = json.load(sys.stdin)
    server = config.get('server', 'https://control:8443')
    jar = http.cookiejar.CookieJar()
    context = ssl.create_default_context(cafile=config.get('ca', '/trust/control.crt'))
    opener = urllib.request.build_opener(urllib.request.HTTPSHandler(context=context), urllib.request.HTTPCookieProcessor(jar))
    csrf = ''
    def call(path, body=None, method='POST'):
        headers = {'Content-Type':'application/json','Origin':server,'X-CSRF-Token':csrf,'Idempotency-Key':uuid.uuid4().hex}
        request = urllib.request.Request(server+'/v2/admin/'+path, data=json.dumps(body).encode() if body is not None else None, headers=headers, method=method)
        with opener.open(request, timeout=15) as response: return json.load(response)
    if config.get('code'): call('bootstrap', {k:config[k] for k in ('code','username','password')})
    csrf = call('login', {k:config[k] for k in ('username','password')})['csrf']
    if config.get('create'):
        node = call('nodes',config['create'])
        result = {'node':node, **call('nodes/'+node['id']+'/key',{})}
    else:
        result = call(config['path'],config.get('body'),config.get('method','POST'))
    print(json.dumps(result))

if __name__ == '__main__': main()
