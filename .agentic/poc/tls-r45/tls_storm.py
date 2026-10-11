"""Non-Go TLS 1.3 handshake storm against the simulator NSX listener (R-45 ③ investigation).
usage: py -3 -I tls_storm.py <host:port> <ca.pem> <seconds> <threads>   (STORM_RST=1: close with RST, STORM_TLS12=1: TLS 1.2)
Each iteration: new TCP connection, TLS 1.3 handshake (SNI nsx.cloudinfra-sim.test), one HTTP GET, close.
"""
import os, socket, ssl, struct, sys, threading, time, collections
RST = os.environ.get('STORM_RST') == '1'  # 클라이언트 TIME_WAIT 회피(SO_LINGER 0): 포트 고갈(WinError 10048) 방지

addr, ca, secs, threads = sys.argv[1], sys.argv[2], float(sys.argv[3]), int(sys.argv[4])
host, port = addr.rsplit(':', 1)
ctx = ssl.create_default_context(cafile=ca)
VER = ssl.TLSVersion.TLSv1_2 if os.environ.get('STORM_TLS12') == '1' else ssl.TLSVersion.TLSv1_3  # STORM_TLS12=1: 대조군
ctx.minimum_version = VER
ctx.maximum_version = VER
counts = collections.Counter()
lock = threading.Lock()
deadline = time.time() + secs

def worker():
    while time.time() < deadline:
        k = 'ok'
        try:
            with socket.create_connection((host, int(port)), timeout=30) as s:
                if RST:
                    s.setsockopt(socket.SOL_SOCKET, socket.SO_LINGER, struct.pack('ii', 1, 0))
                with ctx.wrap_socket(s, server_hostname='nsx.cloudinfra-sim.test') as t:
                    t.sendall(b'GET /api/v1/node/version HTTP/1.1\r\nHost: nsx.cloudinfra-sim.test\r\nConnection: close\r\n\r\n')
                    data = t.recv(64)
                    if not data.startswith(b'HTTP/1.1'):
                        k = 'bad response'
        except Exception as e:
            k = type(e).__name__ + ': ' + str(e)[:80]
        with lock:
            counts[k] += 1

ts = [threading.Thread(target=worker) for _ in range(threads)]
for t in ts: t.start()
for t in ts: t.join()
for k, v in counts.most_common():
    print(v, k)
print('total', sum(counts.values()))
