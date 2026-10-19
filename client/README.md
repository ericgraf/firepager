# RTL-SDR calibration and tuning

The client is receive-only: `rtl_fm` feeds `multimon-ng` and no transmit
utility is used.  Each dongle can have a different crystal offset and RF
gain. Configure these variables in the client container:

* `SDR_PPM` — crystal correction in ppm (default `0`).
* `SDR_GAIN` — RTL gain in dB (default `49`). Use a value supported by the
  dongle; calibrate and operate dongles at the same gain when comparing them.
* `SDR_SAMPLE_RATE` — rtl_fm sample rate (default `22050`).
* `PAGER_FREQ` — pager frequency in Hz; defaults to `155610000` (155.610 MHz,
  the Ottawa Fire paging allocation listed in ISED TAFL data).
* `SDR_CALIBRATE_STATIONS` — comma-separated `name=frequency` FM stations;
  calibration is disabled when empty.
* `SDR_CALIBRATE_ON_START` — set `true` to calibrate before starting POCSAG.
* `SDR_CALIBRATE_INTERVAL` — periodic interval in seconds; `0` (default)
  disables periodic calibration.
* `SDR_CALIBRATE_JITTER` — random positive jitter in seconds added to each
  interval (default `0`).
* `SDR_CALIBRATE_MIN_STATIONS` — minimum number of agreeing references required
  before committing a result (default `1`; use `2` for unattended operation).
* `SDR_CALIBRATE_QUIET_PERIOD` — seconds without decoder activity before a
  periodic scan may take ownership of the dongle (default `5`).
* `SDR_CALIBRATION_FILE` — last-known-good JSON path (default
  `/var/lib/firepager/sdr-calibration.json`).
* `SDR_CALIBRATE_TARGET_ON_START` — calibrate around `PAGER_FREQ` using the
  pager signal itself (enabled by default). Set `false` only when the startup
  receive interruption is unacceptable.
* `SDR_CALIBRATE_TARGET_PPM_MIN` / `MAX` / `STEP` — candidate range (defaults
  `-8`, `8`, and `1` ppm).
* `SDR_CALIBRATE_TARGET_REPEATS` — complete candidate sweeps required for
  repeatability (default `2`). Minimum prominence and margin default to 6 dB
  and 1 dB respectively.
* `SDR_DEVICE` / `SDR_SERIAL` — optional stable dongle identity used to share
  the OS lock between clients and calibration commands. `SDR_LOCK_PATH` can
  override the lock file; otherwise a hash of the identity (or spool path) is
  stored under `/tmp`.

Calibration interval and jitter must be non-negative numbers, and
`SDR_CALIBRATE_MIN_STATIONS` must be a positive integer. Invalid calibration
environment values are rejected at client startup; correct them rather than
letting the client run with an unverified correction.

## Calibrate against FM broadcast

On the Pi, with the antenna connected and the client stopped, run for one or
more nearby stations (frequency may be MHz or Hz):

```sh
python3 /app/sdr_calibrate.py --station 'local=99.9' --station 'news=101.1' \
  --gain 38 --output /var/lib/firepager/sdr-calibration.json
```

The command uses `rtl_power` for a short scan around each carrier and prints
the median `SDR_PPM`, the chosen `SDR_GAIN`, and per-station measurements.
Set the two values as container environment variables (or copy them from the
JSON file) before starting FirePager. A positive reported PPM means the
strongest measured bin was above the published frequency; `-p` is passed to
`rtl_fm` as the correction. The JSON also records the observed offset and an
opposite-sign candidate correction; hardware-test the installed rtl-sdr build
before changing the deployed sign.

## Calibrate against the pager target

The target-band path is receive-only and can be run with the client stopped:

```sh
python3 /app/sdr_calibrate.py --target "$PAGER_FREQ" --gain 49 \
  --ppm-min -8 --ppm-max 8 --repeats 2 \
  --output /var/lib/firepager/sdr-calibration.json
```

Target calibration runs by default. Each candidate is scanned repeatedly with
`rtl_power`; weak, absent, or ambiguous signals do not replace the configured
or last-known-good PPM. Startup calibration runs before `rtl_fm` starts, and an
OS-level lock is held for the decoder lifetime (and reacquired for every scan),
so separate client/calibration processes cannot share one dongle. Device-open
failures are retried with bounded backoff; signal-quality failures retain the
last-known-good values.

## Periodic calibration

For unattended clients, set `SDR_CALIBRATE_STATIONS`,
`SDR_CALIBRATE_ON_START=true`, an interval, and optional jitter. For example,
`SDR_CALIBRATE_INTERVAL=86400 SDR_CALIBRATE_JITTER=3600` runs roughly once a
day, with up to an hour of positive randomization. The scheduler uses an
injected monotonic clock internally; the client also holds a calibration lock,
so concurrent maintenance requests cannot run two scans at once.
Because `rtl_power` and `rtl_fm` cannot share one dongle, the current decoder
process is stopped only at the scheduled maintenance boundary, calibration is
bounded by the scanner timeout, and the decoder is restarted immediately.
**Any in-band calibration causes a receive outage:** POCSAG pages cannot be
decoded while `rtl_power` owns the dongle, including during startup calibration
and each periodic maintenance scan. Schedule it during an acceptable outage
window and monitor the client after the decoder restarts.
Successful results are atomically written to the calibration file. A failed
run keeps the previous values and exponentially backs off (up to eight base
intervals), so a bad station does not continuously interrupt POCSAG capture.
Reference estimates must agree within 3 ppm and remain within the 200 ppm
safety range; set `SDR_CALIBRATE_MIN_STATIONS=2` to require independent
references.
If no previous file exists, the explicitly configured `SDR_PPM`/`SDR_GAIN`
remain in effect.

The station should be strong and local, and its published frequency should be
accurate. FM multipath, coarse scan bins, interference, and a weak antenna can
produce bad results; repeat with another station if values disagree. This is
not an absolute gain calibration: RTL power readings are relative. `SDR_GAIN`
is intentionally explicit so each dongle's usable gain can be selected and
recorded without risking overload. Never use this tool for transmission.
