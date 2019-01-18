#!/usr/bin/env python2
import os, time, subprocess, requests, sys, syslog

RECONFIG_CMD = "v4l2-ctl --set-fmt-video=width=1920,height=1080,pixelformat=1".split(" ")
PRIME_CMD = "v4l2-ctl --stream-to=- --stream-mmap=3 --stream-count=60".split(" ")
CAPTURE_CMD = "v4l2-ctl --stream-to=- --stream-mmap=3 --stream-count=1".split(" ")
BUS_POWER_FILE = "/sys/devices/platform/soc/3f980000.usb/buspower"
CAPTURE_INTERVAL = 5.
HEADERS = dict([[j.strip() for j in i.strip().split(":") if j.strip()] for i in os.environ.get("HEADERS", "").split(";") if i.strip()])

URL = len(sys.argv) > 1 and sys.argv[1] or "http://localhost:9000/upsydaisy"

def reconfig():
  syslog.syslog("power cycling camera")
  f = file(BUS_POWER_FILE, "w")
  f.write("0")
  f.close()
  f = file(BUS_POWER_FILE, "w")
  f.write("1")
  f.close()
  time.sleep(2.)
  subprocess.call(RECONFIG_CMD, stderr=None)
  subprocess.call(PRIME_CMD, stderr=None)

if __name__ == "__main__":
  count = 0
  devnull = file("/dev/null","w")
  HEADERS['content-type'] = 'image/jpeg'
  while True:
    start = time.time()
    try:
      jpeg = subprocess.check_output(CAPTURE_CMD, stderr=devnull)
      try:
        res = requests.post(URL, data=jpeg, headers=HEADERS, timeout=0.1)
      except:
        syslog.syslog(str(sys.exc_info()[1]))

      if not count % 500:
        reconfig()
    except KeyboardInterrupt:
      raise SystemExit(0)
    except:
      syslog.syslog(str(sys.exc_info()[1]))

    d = time.time() - start
    d = d > 5. and 5. or d
    d = d < 0. and 0. or d
    time.sleep(5. - d)

    count += 1
