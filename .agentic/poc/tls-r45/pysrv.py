"""Non-Go TLS 1.3 server (OpenSSL via Python ssl) for R-45 ③ control: same storm, no Go on either side.
usage: py -3 -I pysrv.py <host:port> <cert-dir>   (writes cert.pem/key.pem/ca.pem with the cryptography-free openssl CLI)
"""
import os, socket, ssl, subprocess, sys, threading, time

addr, d = sys.argv[1], sys.argv[2]
host, port = addr.rsplit(':', 1)
os.makedirs(d, exist_ok=True)
cert, key = os.path.join(d, 'cert.pem'), os.path.join(d, 'key.pem')
subprocess.run(['openssl', 'req', '-x509', '-newkey', 'ec', '-pkeyopt', 'ec_paramgen_curve:P-256', '-nodes', '-days', '1',
                '-subj', '/CN=nsx.cloudinfra-sim.test', '-addext', 'subjectAltName=DNS:nsx.cloudinfra-sim.test',
                '-keyout', key, '-out', cert], check=True, capture_output=True)
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
ctx.minimum_version = ssl.TLSVersion.TLSv1_2
ctx.load_cert_chain(cert, key)
errors = {}
lock = threading.Lock()


def handle(conn):
    try:
        with ctx.wrap_socket(conn, server_side=True) as t:
            t.recv(4096)
            t.sendall(b'HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok')
    except Exception as e:
        k = type(e).__name__ + ': ' + str(e)[:60]
        with lock:
            errors[k] = errors.get(k, 0) + 1


def report():
    while True:
        time.sleep(10)
        with lock:
            print('server errors', dict(errors), flush=True)


threading.Thread(target=report, daemon=True).start()
ls = socket.socket()
ls.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
ls.bind((host, int(port)))
ls.listen(1024)
print('listening', addr, flush=True)
while True:
    c, _ = ls.accept()
    threading.Thread(target=handle, args=(c,), daemon=True).start()
