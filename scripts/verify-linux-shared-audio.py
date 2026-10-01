import configparser, hashlib, json, math, os, re, socket, struct, subprocess, tempfile, time, sys
from pathlib import Path

children=[]
display=int(sys.argv[1]) if len(sys.argv)>1 else 2
test_seconds=int(sys.argv[2]) if len(sys.argv)>2 else 3
def start(args, **kw):
    p=subprocess.Popen(args,**kw);children.append(p);return p
def stop(p):
    if p.poll() is None:
        p.terminate()
        try:p.wait(timeout=5)
        except subprocess.TimeoutExpired:p.kill();p.wait()
def run(args, **kw):
    return subprocess.run(args,check=True,capture_output=True,**kw).stdout
def tone(seconds=None):
    seconds=test_seconds if seconds is None else seconds
    return b''.join(struct.pack('<hh',int(5000*math.sin(2*math.pi*997*i/44100)),int(5000*math.sin(2*math.pi*997*i/44100))) for i in range(44100*seconds))
def metrics(data):
    data=data[:len(data)//4*4]
    a=struct.unpack('<'+'h'*(len(data)//2),data)[::2]
    nonzero=[v for v in a if abs(v)>50]
    crossings=sum(a[i]<=0<a[i+1] for i in range(len(a)-1))
    indices=[i for i,v in enumerate(a) if abs(v)>50]
    duration=(indices[-1]-indices[0]+1)/44100 if indices else 0
    # Estimate pitch from individual cycles, separately from transport gaps.
    edges=[i for i in range(len(a)-1) if a[i]<=0<a[i+1]]
    periods=[b-a for a,b in zip(edges,edges[1:])]
    valid=[p for p in periods if 30<=p<=60]
    frequency=round(44100*len(valid)/sum(valid),1) if valid else 0
    longest=0;current=0;gaps=[]
    for i,v in enumerate(a[indices[0]:indices[-1]+1] if indices else []):
        if abs(v)>=50 and current>441:
            gaps.append({'start_s':round((i-current)/44100,3),'length_ms':round(current/44.1,2)})
        current=current+1 if abs(v)<50 else 0
        longest=max(longest,current)
    return {'frames':len(a),'nonzero_frames':len(nonzero),'rms':round(math.sqrt(sum(v*v for v in a)/max(1,len(a))),1),'tone_hz':frequency,'tone_seconds':round(duration,2),'longest_internal_silence_ms':round(longest/44.1,2),'gaps':gaps[:10],'pitch_and_delivery_passed':duration>2.5 and abs(frequency-997)<5}

with tempfile.TemporaryDirectory(prefix='pfremote-audio-test-') as tmp:
    root=Path(tmp); report={}
    try:
        # Reuse the credential of this exact existing VNC instance, only in memory.
        av=None
        for proc in Path('/proc').iterdir():
            if not proc.name.isdigit():continue
            try:
                args=(proc/'cmdline').read_bytes().decode().split('\0')
                if args and Path(args[0]).name=='Xvnc' and ':'+str(display) in args:
                    av=args;original_pid=proc.name;break
            except (OSError,UnicodeError):pass
        assert av is not None
        targetenv=os.environ.copy();targetenv['DISPLAY']=':'+str(display)
        geometry=re.search(rb'dimensions:\s+(\d+x\d+)',run(['xdpyinfo'],env=targetenv)).group(1).decode()
        before=hashlib.sha256(run(['xprop','-root','_NET_CLIENT_LIST'],env=targetenv)).hexdigest()
        encrypted=Path(av[av.index('-PasswordFile')+1]).read_bytes()[:8]
        secret=run(['openssl','enc','-d','-des-ecb','-K','e84ad660c4721ae0','-nopad','-provider','legacy'],input=encrypted).rstrip(b'\0')
        conf=configparser.ConfigParser(interpolation=None,strict=False);conf.read('/etc/xrdp/xrdp.ini')
        cert=conf.get('Globals','certificate',fallback='') or '/etc/xrdp/cert.pem'
        fingerprint=hashlib.sha256(run(['sudo','-n','openssl','x509','-in',cert,'-outform','DER'])).hexdigest()
        fingerprint=':'.join(fingerprint[i:i+2] for i in range(0,len(fingerprint),2))
        d=next(d for d in range(90,110) if not Path('/tmp/.X11-unix/X'+str(d)).exists())
        auth=root/'Xauthority';auth.touch(mode=0o600)
        run(['xauth','-f',str(auth),'add',':'+str(d),'MIT-MAGIC-COOKIE-1',os.urandom(16).hex()])
        env=os.environ.copy();env.update(DISPLAY=':'+str(d),XAUTHORITY=str(auth))
        start(['Xvfb',':'+str(d),'-screen','0','1920x1080x24','-nolisten','tcp','-auth',str(auth)],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
        time.sleep(1)
        # Dedicated virtual devices: neither the real microphone nor speakers are used.
        for name in ['pfremote-test-output','pfremote-test-input']:
            start(['pw-cli','-m','create-node','adapter',f'{{ factory.name=support.null-audio-sink node.name={name} media.class=Audio/Sink audio.position=[FL FR] priority.session=0 object.linger=false }}'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
        time.sleep(1)
        with (root/'client.log').open('wb') as log:
            client=start(['xfreerdp','/v:127.0.0.1','/u:na','/d:pfremote-shared-'+str(display),'/from-stdin:force','/sec:tls','/cert:fingerprint:sha256:'+fingerprint,'/size:'+geometry,'/sound:sys:pulse,dev:pfremote-test-output,latency:200','/microphone:sys:pulse,dev:pfremote-test-input.monitor','-clipboard','/log-level:ERROR'],env=env,stdin=subprocess.PIPE,stdout=log,stderr=subprocess.STDOUT)
            client.stdin.write(secret+b'\n');client.stdin.flush();secret=b''
            time.sleep(5)
            report['rdp_client_alive']=client.poll() is None
            report['display']=display
            report['audio_channel_socket']=Path('/run/xrdp/'+str(os.getuid())+'/xrdp_chansrv_audio_out_socket_'+str(display)).exists()
            raw=['--raw','--rate=44100','--channels=2','--format=s16']
            for direction,playtarget,rectarget,monitor in [('playback','pfremote-speaker-'+str(display),'pfremote-test-output',True),('microphone','pfremote-test-input','pfremote-mic-'+str(display),False)]:
                targetfile=root/(direction+'.raw')
                with targetfile.open('wb') as out:
                    args=['pw-cat','--record','--target',rectarget,*raw]
                    if monitor:args+=['-P','stream.capture.sink=true']
                    recorder=start(args+['-'],stdout=out,stderr=subprocess.DEVNULL)
                    time.sleep(.4)
                    play=start(['pw-cat','--playback','--target',playtarget,*raw,'-'],stdin=subprocess.PIPE,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
                    try:play.communicate(tone(),timeout=test_seconds+5)
                    except subprocess.TimeoutExpired:stop(play)
                    time.sleep(.4);stop(recorder)
                report[direction]=metrics(targetfile.read_bytes())
            stop(client)
        logs=(root/'client.log').read_text(errors='replace')
        report['client_error_codes']=sorted(set(re.findall(r'ERRCONNECT_[A-Z_]+|STATUS_[A-Z_]+',logs)))
        report['same_xserver_alive']=Path('/proc/'+original_pid).exists()
        report['window_list_unchanged']=before==hashlib.sha256(run(['xprop','-root','_NET_CLIENT_LIST'],env=targetenv)).hexdigest()
        # No raw logs, password, host identity or audio data leaves this test.
    except Exception as e:
        report['error_type']=type(e).__name__
        if isinstance(e,subprocess.CalledProcessError):
            report['failed_program']=e.cmd[0]
            report['returncode']=e.returncode
            report['error_hint']=re.sub(r'/[^\s:]+','[path]',(e.stderr or b'').decode(errors='replace'))[:500]
    finally:
        for p in reversed(children):stop(p)
    report['passed']=bool(report.get('rdp_client_alive') and report.get('same_xserver_alive') and report.get('window_list_unchanged') and not report.get('client_error_codes') and all(report.get(k,{}).get('pitch_and_delivery_passed') and not report.get(k,{}).get('gaps') for k in ['playback','microphone']))
    print(json.dumps(report))
    sys.exit(0 if report['passed'] else 1)

