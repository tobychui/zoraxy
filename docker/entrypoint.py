#!/usr/bin/env python3

import os
import signal
import subprocess
import sys
import time

zoraxy_proc = None
zerotier_proc = None

def getenv(key, default=None):
  return os.environ.get(key, default)

def run(command):
  try:
    subprocess.run(command, check=True)
  except subprocess.CalledProcessError as e:
    print(f"Command failed: {command} - {e}")
    sys.exit(1)

def popen(command):
  proc = subprocess.Popen(command)
  time.sleep(1)
  if proc.poll() is not None:
    print(f"{command} exited early with code {proc.returncode}")
    raise RuntimeError(f"Failed to start {command}")
  return proc

def cleanup(_signum, _frame):
  print("Shutdown signal received. Cleaning up...")

  global zoraxy_proc, zerotier_proc

  if zoraxy_proc and zoraxy_proc.poll() is None:
    print("Terminating Zoraxy...")
    zoraxy_proc.terminate()

  if zerotier_proc and zerotier_proc.poll() is None:
    print("Terminating ZeroTier-One...")
    zerotier_proc.terminate()

  if zoraxy_proc:
    try:
      zoraxy_proc.wait(timeout=8)
    except subprocess.TimeoutExpired:
      zoraxy_proc.kill()
      zoraxy_proc.wait()

  if zerotier_proc:
    try:
      zerotier_proc.wait(timeout=8)
    except subprocess.TimeoutExpired:
      zerotier_proc.kill()
      zerotier_proc.wait()

  try:
    os.unlink("/var/lib/zerotier-one")
  except FileNotFoundError:
    pass
  except Exception as e:
    print(f"Failed to unlink ZeroTier socket: {e}")

  sys.exit(0)

def start_zerotier():
  print("Starting ZeroTier...")

  global zerotier_proc

  config_dir = "/opt/zoraxy/config/zerotier/"
  zt_path = "/var/lib/zerotier-one"

  os.makedirs(config_dir, exist_ok=True)

  try:
    os.symlink(config_dir, zt_path, target_is_directory=True)
  except FileExistsError:
    print(f"Symlink {zt_path} already exists, skipping creation.")

  zerotier_proc = popen(["zerotier-one"])

def start_zoraxy():
  print("Starting Zoraxy...")

  global zoraxy_proc

  zoraxy_args = [
    "zoraxy",
    f"-autorenew={ getenv('AUTORENEW', '86400') }",
    f"-cfgupgrade={ getenv('CFGUPGRADE', 'true') }",
    f"-db={ getenv('DB', 'auto') }",
    f"-docker={ getenv('DOCKER', 'true') }",
    f"-earlyrenew={ getenv('EARLYRENEW', '30') }",
    f"-enablelog={ getenv('ENABLELOG', 'true') }",
    f"-fastgeoip={ getenv('FASTGEOIP', 'false') }",
    f"-mdns={ getenv('MDNS', 'true') }",
    f"-mdnsname={ getenv('MDNSNAME', "''") }",
    f"-noauth={ getenv('NOAUTH', 'false') }",
    f"-plugin={ getenv('PLUGIN', '/opt/zoraxy/plugin/') }",
    f"-port=:{ getenv('PORT', '8000') }",
    f"-sshlb={ getenv('SSHLB', 'false') }",
    f"-update_geoip={ getenv('UPDATE_GEOIP', 'false') }",
    f"-version={ getenv('VERSION', 'false') }",
    f"-webfm={ getenv('WEBFM', 'true') }",
    f"-webroot={ getenv('WEBROOT', './www') }",
  ]

  zoraxy_proc = popen(zoraxy_args)

def main():
  signal.signal(signal.SIGTERM, cleanup)
  signal.signal(signal.SIGINT, cleanup)

  print("Updating CA certificates...")
  run(["update-ca-certificates"])

  print("Updating GeoIP data...")
  run(["zoraxy", "-update_geoip=true"])

  os.chdir("/opt/zoraxy/config/")

  if getenv("ZEROTIER", "false") == "true":
    start_zerotier()

  start_zoraxy()

  signal.pause()

if __name__ == "__main__":
  main()

