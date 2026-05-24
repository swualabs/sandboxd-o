#!/usr/bin/env python3
import json, time, urllib.request, pathlib, csv
from datetime import datetime

ORCH='http://127.0.0.1:8082'
LOG_DIR=pathlib.Path('.perf/logs')
OUT_DIR=pathlib.Path('.perf/results/resource-sweep-full-'+datetime.now().strftime('%Y%m%d-%H%M%S'))
OUT_DIR.mkdir(parents=True, exist_ok=True)

LEVELS=[('50m','64Mi'),('100m','128Mi'),('200m','256Mi'),('500m','512Mi'),('1000m','1Gi'),('2000m','2Gi'),('3500m','8Gi')]
ITER=5
PREFIX='perf-sweep2'


def req(method, path, body=None):
    data=None; headers={}
    if body is not None:
        data=json.dumps(body).encode(); headers['Content-Type']='application/json'
    r=urllib.request.Request(ORCH+path,data=data,method=method,headers=headers)
    with urllib.request.urlopen(r, timeout=20) as resp:
        return json.loads(resp.read().decode())

def get_phase(sid):
    return req('GET', f'/api/v1/sandboxes/{sid}')['sandbox']['status']['phase']

def delete_sid(sid):
    try:req('DELETE',f'/api/v1/sandboxes/{sid}')
    except:pass

def read_perf(sid):
    found=None
    files=sorted(LOG_DIR.glob('sandboxd-*.log'))
    for fp in files:
        raw=fp.read_bytes().replace(b'\x00', b'')
        for line in raw.splitlines():
            if b'"msg":"perf.provision_sandbox"' not in line:
                continue
            try: obj=json.loads(line.decode('utf-8','ignore'))
            except: continue
            if obj.get('sandbox')==sid:
                found=obj
    return found

rows=[]
for cpu,mem in LEVELS:
    for i in range(ITER):
        sid=f'{PREFIX}-{cpu}-{mem}-{int(time.time()*1000)}-{i}'
        body={"id":sid,"spec":{"egress":False,"ports":[{"container_port":80,"protocol":"tcp"}],"containers":[{"name":"nginx","image":"nginx:latest","resource":{"cpu":cpu,"memory":mem,"ephemeral_storage":"128Mi"}}]}}
        t0=time.time(); req('POST','/api/v1/sandboxes',body); t1=time.time()
        phase='Pending'
        for _ in range(240):
            phase=get_phase(sid)
            if phase in ('Running','Failed','Error'): break
            time.sleep(0.1)
        t2=time.time()

        perf=None
        for _ in range(30):
            perf=read_perf(sid)
            if perf: break
            time.sleep(0.2)

        r={'sandbox_id':sid,'cpu':cpu,'memory':mem,'phase':phase,'create_http_ms':(t1-t0)*1000,'client_ready_ms':(t2-t0)*1000}
        if perf:
            for k,v in perf.items():
                if isinstance(v,(int,float)):
                    r[k+'_ms']=v/1_000_000
        rows.append(r)
        delete_sid(sid); time.sleep(0.2)
        print(f'[{cpu},{mem}] {i+1}/{ITER} phase={phase} client={r["client_ready_ms"]:.1f} total={r.get("total_ms")}', flush=True)

raw=OUT_DIR/'raw.csv'
keys=sorted({k for row in rows for k in row.keys()})
with raw.open('w',newline='') as f:
    w=csv.DictWriter(f,fieldnames=keys); w.writeheader(); w.writerows(rows)

metrics=['client_ready_ms','total_ms','pod_sandbox_create_total_ms','container_create_start_total_ms','wait_published_tcp_ready_ms','container_image_pull_ms','network_policy_apply_ms']
agg=[]
for cpu,mem in LEVELS:
    s=[r for r in rows if r['cpu']==cpu and r['memory']==mem and r['phase']=='Running']
    rec={'cpu':cpu,'memory':mem,'runs':len(s)}
    for m in metrics:
        vals=sorted([r[m] for r in s if m in r])
        if not vals: continue
        rec[m+'_mean']=sum(vals)/len(vals)
        rec[m+'_p50']=vals[int((len(vals)-1)*0.5)]
        rec[m+'_p95']=vals[int((len(vals)-1)*0.95)]
    agg.append(rec)

aggp=OUT_DIR/'agg.csv'
with aggp.open('w',newline='') as f:
    w=csv.DictWriter(f,fieldnames=sorted({k for row in agg for k in row.keys()})); w.writeheader(); w.writerows(agg)

try:
    import pandas as pd
    import matplotlib.pyplot as plt

    adf = pd.read_csv(aggp)
    labels = [f"{c}\n{m}" for c, m in zip(adf["cpu"], adf["memory"])]
    x = range(len(labels))

    fig, ax = plt.subplots(figsize=(12, 6))
    if "total_ms_mean" in adf:
        ax.plot(x, adf["total_ms_mean"], marker="o", label="sbxlet total mean")
    if "total_ms_p95" in adf:
        ax.plot(x, adf["total_ms_p95"], marker="o", label="sbxlet total p95")
    if "wait_published_tcp_ready_ms_mean" in adf:
        ax.plot(x, adf["wait_published_tcp_ready_ms_mean"], marker="o", label="tcp readiness mean")
    ax.set_xticks(list(x))
    ax.set_xticklabels(labels)
    ax.set_ylabel("ms")
    ax.set_title("Resource Limit Sweep: Internal Provisioning Latency")
    ax.grid(True, alpha=0.3)
    ax.legend()
    fig.tight_layout()
    fig.savefig(OUT_DIR / "resource_sweep_internal.png", dpi=160)

    fig2, ax2 = plt.subplots(figsize=(12, 6))
    if "client_ready_ms_mean" in adf:
        ax2.plot(x, adf["client_ready_ms_mean"], marker="o", label="client ready mean")
    if "client_ready_ms_p95" in adf:
        ax2.plot(x, adf["client_ready_ms_p95"], marker="o", label="client ready p95")
    ax2.set_xticks(list(x))
    ax2.set_xticklabels(labels)
    ax2.set_ylabel("ms")
    ax2.set_title("Resource Limit Sweep: Client-visible Latency (sbxorch phase)")
    ax2.grid(True, alpha=0.3)
    ax2.legend()
    fig2.tight_layout()
    fig2.savefig(OUT_DIR / "resource_sweep_client.png", dpi=160)
except Exception as e:
    print("WARN graph generation skipped:", e)

print('OUT', OUT_DIR)
print('RAW', raw)
print('AGG', aggp)
