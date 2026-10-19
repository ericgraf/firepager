import decode_raw_pages
import json

lines = [
    "POCSAG512: Address:  930032  Function: 2  Alpha:   ID:gdsJ3D<LF>SMOKE VISIBLE INSIDE-FIRE <LF>15:55:48<LF>6525 BANK ST  Dv: 9  Sec: 91D <LF>Gd: 91D<LF>CENOTE RD & STONE SCHOOL RD<LF>PT91 PT93 P92 LA91 TA92 TA94<LF>TA83 SC90 #36719<LF>Sent by: FDP2<EOT><NUL>",
    "POCSAG512: Address:  910000  Function: 3  Alpha:   ID:IMzuFU<LF>2ND PAGE STRUCTURE FIRE AT 6525<LF>BANK ST<LF>Sent by: FDP1<EOT><NUL>",
    "POCSAG512: Address:  630000  Function: 3  Alpha:   ID:lTOVIC<LF>WATER/ICE-RESCUE  00:21:57<LF>872 BAYVIEW DR  Dv: 6  Sec: 63B<LF> Gd: 63B<LF>DORIS CURRIE RD & RITCHIE AVE<LF>PT63 TA63 WR62 WR22 SO23 SC90<LF>#37043<LF>Sent by: FDP1<EOT>",
    "POCSAG512: Address:    1296  Function: 2  Alpha:   ID:0VFZuQ<LF>MVC-ROLLOVER  16:35:53<LF>3168 DALMAC RD  Dv: 9  Sec: 92B<LF> Gd: 92B<LF>FOREST RD & SPRINGHILL RD<LF>P92 RE93 SO23 TA92 SC90 #37196<LF>Sent by: FDP1<EOT>",
    "POCSAG512: Address:    1024  Function: 2  Alpha:   ID:4C1aE7<LF>VEHICLE FIRE-EXP.  23:24:42<LF>PRESLAND RD / WHITTON CRES  Dv:<LF>5  Sec: 56F  Gd: 56F<LF>225-237 PRESLAND RD<LF>P56 P57 P13A LA13 DC50 #37238<LF>Sent by: FDP2<EOT><NUL><NUL>"
]

for i, line in enumerate(lines):
    print(f"\n--- Sample {i+1} ---")
    print(f"RAW: {line}")
    decoded = decode_raw_pages.process(line)
    print(f"DECODED: {json.dumps(decoded, indent=2)}")
