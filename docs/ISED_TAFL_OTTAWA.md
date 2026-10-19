# ISED TAFL findings for Ottawa-area SDR calibration

**Checked:** 2026-10-19

## Primary pager allocation

The current ISED Spectrum Management System land-mobile extract lists Ottawa
Fire paging transmitters and pager receivers at **155.610 MHz**. The Ottawa
entries include these sites:

- G9013 North Dundas Water Tower
- E0554 Richmond Drop
- E1105 Beckett's Landing Drop
- E1138 Almonte Drop
- G9002 Randal Drop
- E0253 Forward Drop
- E0453 March Road Drop
- E0221 Cumberland Drop
- E0208 Gloucester Drop
- G9027 Backup Dispatch Drop
- West Carleton

The extract identifies these as `LMR-DIGITAL` and includes Ottawa Fire Services
and pager-unit TX/RX records. This supports using **155610000 Hz** as the
default `PAGER_FREQ`; it does not by itself establish the RTL-SDR's PPM
correction.

## Other Ottawa-area licensed frequencies

The same land-mobile extract contains other Ottawa/National Capital Area
frequencies that may be useful for spectrum investigation:

| Frequency | Example licensed description | Calibration suitability |
|---:|---|---|
| 162.550 MHz | Ottawa, 2001 Carling Ave | Potential weather reference, but verify that it is still transmitting; Weatheradio service status is uncertain. |
| 163.260 MHz | National Capital Area | Licensed land-mobile service; likely intermittent. |
| 163.680 MHz | Ottawa / 53 Elgin St | Licensed land-mobile service; not a guaranteed continuous carrier. |
| 165.030 MHz | Ottawa / 1 Hunt Club Road | Licensed land-mobile service; intermittent traffic expected. |
| 167.565 MHz | Greater Ottawa Area | Licensed portable/mobile service; poor absolute reference unless a continuous signal is confirmed. |
| 168.930 MHz | Ottawa / 4837 Albion Road | Licensed land-mobile service; verify on-air use. |
| 169.680 MHz | Ottawa / 1 Hunt Club Road | Licensed land-mobile service; intermittent traffic expected. |
| 173.760 MHz | 240 McLeod St, Ottawa | Licensed land-mobile service; verify on-air use. |

These are **licensed allocations, not guaranteed calibration beacons**. A
channel is suitable only if a stable, known-frequency carrier is actually
present during calibration. Sporadic voice/data bursts can produce a false
frequency peak.

## Recommended calibration hierarchy

1. Use the licensed **155.610 MHz** pager channel when valid POCSAG traffic is
   present and require repeatable decoded/spectral evidence.
2. Use a continuously transmitting, independently known reference (for
   example a verified weather or broadcast carrier) if available locally.
3. Use a second SDR only as a relative monitor unless one receiver is already
   independently calibrated.
4. Keep the last-known-good PPM when the reference is absent, intermittent, or
   contradictory.

## Data sources

- [ISED SMS data downloads](https://ised-isde.canada.ca/site/spectrum-management-system/en/download-sms-data)
- [Land Mobile Service extract](https://www.ic.gc.ca/engineering/SMS_TAFL_Files/TAFL_LandMobile-LTAF_MobileTerrestre.zip)
- [ISED Spectrum Management System](https://ised-isde.canada.ca/site/spectrum-management-system/en/spectrum-management-system)

The downloaded land-mobile extract was dated **2026-10-05**. The geographic
review used Ontario records within approximately 0.8 degrees latitude and 1.0
degree longitude of Ottawa; it was not a complete legal or engineering survey.

## Host-side RF check

On 2026-10-19 the attached RTL-SDR was scanned around each candidate at
38.6 dB gain. The following peaks were repeatable across three short scans:

| Licensed frequency | Observed peak | Prominence | Interpretation |
|---:|---:|---:|---|
| 155.610 MHz | 155.610858 MHz | ~10 dB | Best candidate; stable Ottawa Fire paging-band signal. A correction near **-5.5 ppm** moved the peak closest to 155.610 MHz in a PPM sweep. |
| 163.260 MHz | 163.269217 MHz | ~13 dB | Stable-looking but 9.2 kHz away; likely another/adjacent signal, not a usable absolute reference without identification. |
| 163.680 MHz | 163.674452 MHz | ~12 dB | Stable-looking but 5.5 kHz away; likely another/adjacent signal. |
| 165.030 MHz | 165.039920 MHz | ~9 dB | Stable-looking but 9.9 kHz away; not accepted as a reference. |
| 173.760 MHz | 173.760233 MHz | ~3–6 dB | Weak; insufficient confidence. |
| 162.550 MHz | 162.552967 MHz | ~2 dB | Too weak/ambiguous for calibration. |
| 167.565 MHz | 167.560624 MHz | ~1 dB | Noise-level. |
| 168.930 MHz | 168.928671 MHz | <1 dB | Noise-level. |
| 169.680 MHz | 169.687108 MHz | <1 dB | Noise-level. |

The 155.610 MHz result is the only candidate currently suitable for practical
testing. It is still based on the licensed pager channel itself, so automatic
correction should be confirmed with valid decoded POCSAG traffic and retained
only when reception improves. Other licensed channels may be intermittent or
may have adjacent-channel activity despite appearing in TAFL.
