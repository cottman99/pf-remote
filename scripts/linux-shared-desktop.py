#!/usr/bin/env python3
"""Provision xrdp 0.10 entries for existing TigerVNC desktops, never new sessions.

Explicit administrator tool, not invoked by the daemon or an update. Requires
the upstream pipewire-module-xrdp module to have been installed first.
"""
import argparse
import configparser
import json
import os
from pathlib import Path
import re
import subprocess
import sys

BEGIN = "# BEGIN PFREMOTE SHARED DESKTOPS"
END = "# END PFREMOTE SHARED DESKTOPS"


def remove_managed_config(current):
    if current.count(BEGIN) != 1 or current.count(END) != 1 or current.index(END) < current.index(BEGIN):
        raise ValueError("managed section changed; manual recovery required")
    pattern = re.escape(BEGIN) + r"[\s\S]*?" + re.escape(END) + r"\n?"
    return re.sub(pattern, "", current)


def render_config(original, uid, displays):
    if not displays or len(displays) != len(set(displays)) or any(d < 1 or d > 99 for d in displays):
        raise ValueError("unique existing display numbers 1..99 required")
    if uid < 1:
        raise ValueError("a non-root desktop owner is required")
    if BEGIN in original or END in original:
        raise ValueError("already configured; rollback before changing the workspace mapping")
    config = configparser.ConfigParser(interpolation=None, strict=False)
    config.read_string(original)
    if config.get("Globals", "security_layer", fallback="").lower() not in ("tls", "negotiate"):
        raise ValueError("existing xrdp must support TLS")
    for key in ("rdpsnd", "drdynvc"):
        if config.get("Channels", key, fallback="").lower() != "true":
            raise ValueError("enable the existing RDP audio channels first")
    parts = [original.rstrip(), "", BEGIN]
    for display in displays:
        section = "pfremote-shared-" + str(display)
        if config.has_section(section):
            raise ValueError("conflicting existing workspace section")
        parts.append(f"""[{section}]
name=Shared desktop :{display} (same as TigerVNC)
lib=libvnc.so
ip=127.0.0.1
port={5900 + display}
username=na
password=ask
chansrvport=DISPLAY({display},{uid})
""")
    parts.append(END)
    return "\n".join(parts) + "\n"


def unit(display, uid, home, audio=False):
    # Paths are inserted in systemd syntax, not shell syntax. Refuse specifiers
    # and quoting characters rather than interpreting an arbitrary home path.
    if not re.fullmatch(r"/[A-Za-z0-9_./-]+", home):
        raise ValueError("unsupported home path")
    command = "/usr/sbin/xrdp-chansrv"
    # Clipboard/device channels must survive an audio-engine update or recovery.
    dependencies = "After=pipewire.service"
    if audio:
        dependencies += f"\nRequires=pipewire.service\nPartOf=pipewire.service\nAfter=pfremote-shared-desktop-{display}.service\nBindsTo=pfremote-shared-desktop-{display}.service"
        # A zero-priority source ties with a sink monitor. Browsers can then
        # select playback loopback as their default microphone before routing.
        # Prefer a real capture source; explicit user defaults still take priority.
        command = f'/usr/bin/pw-cli -m -d load-module libpipewire-module-xrdp "sink.stream.props={{ node.always-process=true node.pause-on-idle=false node.name=pfremote-speaker-{display} node.description=\\"Desktop {display} speakers\\" priority.session=0 }}" "source.stream.props={{ node.name=pfremote-mic-{display} node.description=\\"Desktop {display} microphone\\" priority.session=100 }}"'
    return f"""[Unit]
Description=Shared desktop {display} RDP channels and audio
{dependencies}

[Service]
Environment=DISPLAY=:{display}
Environment=XAUTHORITY={home}/.Xauthority
Environment=XRDP_SOCKET_PATH=/run/xrdp/{uid}
Environment=XRDP_PULSE_SINK_SOCKET=xrdp_chansrv_audio_out_socket_{display}
Environment=XRDP_PULSE_SOURCE_SOCKET=xrdp_chansrv_audio_in_socket_{display}
ExecStart={command}
Type=simple
TimeoutStartSec=10
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
"""


def router_unit(home, displays):
    if not re.fullmatch(r"/[A-Za-z0-9_./-]+", home):
        raise ValueError("unsupported home path")
    return f"""[Unit]
Description=Route shared-workspace application audio
After=pipewire.service wireplumber.service
Wants=pipewire.service wireplumber.service

[Service]
ExecStart=/usr/bin/python3 {home}/.local/share/pfremote/shared-desktop-setup/linux-workspace-audio-router.py --displays {' '.join(map(str, displays))}
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
"""


def input_unit(home, display):
    if not re.fullmatch(r"/[A-Za-z0-9_./-]+", home) or not 1 <= display <= 99:
        raise ValueError("unsupported owner or display")
    return f"""[Unit]
Description=Restore clicked workspace {display} input after focus loss
StartLimitIntervalSec=0

[Service]
Environment=XAUTHORITY={home}/.Xauthority
ExecStart=/usr/bin/python3 {home}/.local/share/pfremote/shared-desktop-setup/linux-workspace-input-guard.py --display {display}
Restart=on-failure
RestartSec=2

[Install]
WantedBy=default.target
"""


def startup_dropin(home, display):
    if not re.fullmatch(r"/[A-Za-z0-9_./-]+", home) or not 1 <= display <= 99:
        raise ValueError("unsupported owner or display")
    return f"""# PF Remote: prepare audio before starting desktop applications.
[Unit]
Wants=pipewire-pulse.socket wireplumber.service pfremote-shared-desktop-{display}-audio.service
After=pipewire-pulse.socket wireplumber.service pfremote-shared-desktop-{display}-audio.service

[Service]
ExecStartPre=/usr/bin/python3 {home}/.local/share/pfremote/shared-desktop-setup/linux-workspace-audio-router.py --prepare-runtime --displays {display} --wait-seconds 20
"""


def main():
    import pwd
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=["plan", "install", "rollback"])
    parser.add_argument("--user", required=True)
    parser.add_argument("--displays", nargs="+", type=int, default=[2, 3, 4])
    parser.add_argument("--desktop-unit-template", help="existing owner desktop unit, e.g. vnc-desktop@.service")
    args = parser.parse_args()
    owner = pwd.getpwnam(args.user)
    uid = owner.pw_uid
    home = owner.pw_dir
    config = Path("/etc/xrdp/xrdp.ini")
    backup = Path("/etc/xrdp/xrdp.ini.pfremote-before-shared")
    units = Path(home) / ".config/systemd/user"
    template = args.desktop_unit_template
    if template is None and (units / 'vnc-desktop@.service').is_file():
        template = 'vnc-desktop@.service'
    if template is not None and (not re.fullmatch(r'[A-Za-z0-9_.-]+@\.service', template) or
                                not (units / template).is_file() or (units / template).is_symlink()):
        raise ValueError('an existing regular owner desktop template is required')
    startup_paths = [(units / (template.replace('@.', '@' + str(d) + '.') + '.d') /
                      '50-pfremote-workspace-ready.conf') for d in args.displays] if template else []
    def userctl(*a):
        subprocess.run(["runuser", "-u", args.user, "--", "env",
                        f"XDG_RUNTIME_DIR=/run/user/{uid}", "systemctl", "--user", *a], check=True)
    names = [f"pfremote-shared-desktop-{d}{suffix}.service"
             for d in args.displays for suffix in ("", "-audio")]
    names.append("pfremote-workspace-audio-router.service")
    names.extend(f"pfremote-workspace-input-{d}.service" for d in args.displays)
    if args.action != "plan" and os.geteuid() != 0:
        raise ValueError("administrator privileges are required")
    if args.action == "rollback":
        if not backup.is_file() or backup.is_symlink():
            raise ValueError("no regular rollback backup")
        # Only remove precisely the owned sections; retain subsequent admin edits.
        current = config.read_text()
        restored = remove_managed_config(current)
        for d, path in zip(args.displays, startup_paths):
            if (path.parent.is_symlink() or path.is_symlink() or
                    (path.exists() and path.read_text() != startup_dropin(home, d))):
                raise ValueError('startup hook has later edits; preserve it for review')
        for name in names:
            if (units / name).exists():
                userctl("disable", "--now", name)
                (units / name).unlink()
        for path in startup_paths:
            path.unlink(missing_ok=True)
        router_path = Path(home) / ".local/share/pfremote/shared-desktop-setup/linux-workspace-audio-router.py"
        if router_path.is_file() and not router_path.is_symlink():
            subprocess.run(["runuser", "-u", args.user, "--", "python3", str(router_path),
                            "--remove-runtime-links", "--displays", *map(str, args.displays)], check=True)
        config.write_text(restored)
        userctl("daemon-reload")
        subprocess.run(["systemctl", "restart", "xrdp"], check=True)
        backup.unlink()
        print(json.dumps({"schema_version": "pfremote.shared-desktop/v1", "state": "rolled-back"}))
        return
    rendered = render_config(config.read_text(), uid, args.displays)
    rendered_units = [unit(d, uid, home, audio) for d in args.displays for audio in (False, True)]
    rendered_units.append(router_unit(home, args.displays))
    rendered_units.extend(input_unit(home, d) for d in args.displays)
    if args.action == "plan":
        print(json.dumps({"schema_version": "pfremote.shared-desktop/v1", "state": "planned",
                          "displays": args.displays, "requires_admin": True,
                          "audio_requires_live_rdp": True, "startup_gate_configured": bool(template)}))
        return
    import shutil
    import socket
    import glob
    router_source = Path(__file__).with_name("linux-workspace-audio-router.py")
    router_path = Path(home) / ".local/share/pfremote/shared-desktop-setup/linux-workspace-audio-router.py"
    guard_source = Path(__file__).with_name('linux-workspace-input-guard.py')
    guard_path = router_path.with_name(guard_source.name)
    if not guard_source.is_file() or guard_source.is_symlink():
        raise ValueError('workspace input guard is missing from the setup bundle')
    for path in startup_paths:
        if path.parent.is_symlink() or path.exists() or path.is_symlink():
            raise ValueError('existing desktop startup hook must not be overwritten')
    if not router_source.is_file() or router_source.is_symlink():
        raise ValueError("workspace audio router is missing from the setup bundle")
    if router_source.resolve() != router_path.resolve():
        if router_path.exists() or router_path.is_symlink():
            raise ValueError("existing workspace audio router must not be overwritten")
        router_path.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(router_source, router_path)
        router_path.chmod(0o644)
    if guard_source.resolve() != guard_path.resolve():
        if guard_path.exists() or guard_path.is_symlink():
            raise ValueError('existing workspace input guard must not be overwritten')
        shutil.copyfile(guard_source, guard_path)
        guard_path.chmod(0o644)
    if not glob.glob("/usr/lib*/pipewire-0.3/libpipewire-module-xrdp.so"):
        raise ValueError("install pipewire-module-xrdp before provisioning")
    if config.is_symlink() or backup.exists() or backup.is_symlink():
        raise ValueError("configuration path or existing backup needs review")
    for name in names:
        if (units / name).exists() or (units / name).is_symlink():
            raise ValueError("existing service must not be overwritten")
    for d in args.displays:
        with socket.create_connection(("127.0.0.1", 5900 + d), timeout=2) as connection:
            if not connection.recv(12).startswith(b"RFB "):
                raise ValueError("workspace is not an existing VNC server")
    units.mkdir(parents=True, exist_ok=True)
    # Use the owner's manager; never start a new login or a new X server.
    userctl("is-active", "pipewire.service")
    shutil.copy2(config, backup)
    written, startup_written = [], []
    try:
        for name, content in zip(names, rendered_units):
            path = units / name
            path.write_text(content)
            os.chown(path, uid, owner.pw_gid)
            written.append(name)
        for d, path in zip(args.displays, startup_paths):
            path.parent.mkdir(parents=True, exist_ok=True)
            os.chown(path.parent, uid, owner.pw_gid)
            path.write_text(startup_dropin(home, d))
            os.chown(path, uid, owner.pw_gid)
            startup_written.append((path, startup_dropin(home, d)))
        userctl("daemon-reload")
        for name in names:
            userctl("enable", "--now", name)
        import time
        time.sleep(2)
        for name in names:
            userctl("is-active", name)
        config.write_text(rendered)
        subprocess.run(["systemctl", "restart", "xrdp"], check=True)
        subprocess.run(["systemctl", "is-active", "xrdp"], check=True)
    except Exception:
        shutil.copy2(backup, config)
        for name in written:
            try:
                userctl("disable", "--now", name)
            except subprocess.CalledProcessError:
                pass
            (units / name).unlink(missing_ok=True)
        for path, content in startup_written:
            if not path.is_symlink() and path.exists() and path.read_text() == content:
                path.unlink()
        userctl("daemon-reload")
        subprocess.run(["systemctl", "restart", "xrdp"], check=False)
        raise
    print(json.dumps({"schema_version": "pfremote.shared-desktop/v1", "state": "configured",
                      "displays": args.displays, "end_to_end_verified": False}))


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        print(type(error).__name__ + ": " + str(error), file=sys.stderr)
        sys.exit(1)
