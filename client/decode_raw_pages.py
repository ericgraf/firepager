import sys
import requests
import re

def find_between(s, first, last):
    try:
        if isinstance(first, int):
            start = first
        else:
            start = s.index(first) + len(first)
            
        if isinstance(last, int):
            end = last
        else:
            end = s.index(last, start)
        return s[start:end]
    except ValueError:
        return ""

def findnth(haystack, needle, n):
    parts = haystack.split(needle, n+1)
    if len(parts) <= n+1:
        return -1
    return len(haystack) - len(parts[-1])

def get_long_lat(address):
    # Stub for geocoding as it requires an API key
    try:
        # url = 'https://maps.googleapis.com/maps/api/geocode/json'
        # params = {'sensor': 'false', 'address': '%s, ON, Canada' % address}
        # r = requests.get(url, params=params)
        # results = r.json()['results']
        # location = results[0]['geometry']['location']
        # return {"longitude": location['lng'], "latitude": location['lat']}
        pass
    except Exception:
        pass
    return {"longitude": 0, "latitude": 0}

def find_address(line):
    if "Dv:" in line:
        return find_between(line, 0, "Dv:").strip()
    return find_between(line, 0, "Dt:").strip()

def check_if_vehicle_inline(line):
    vehicle_types = ["P", "PT", "TA", "WR", "SC", "LA", "DC", "RE", "SO"]
    tmp = line.split()
    for x in tmp:
        for v in vehicle_types:
            if x.lower().startswith(v.lower()):
                num_part = x.lower().replace(v.lower(), "")
                if num_part.isdigit():
                    return True
    return False

def find_vehicles(line, before):
    first = ""
    if check_if_vehicle_inline(before):
        first = before + " "
    
    vehicles_part = find_between(line, 0, "#").strip()
    if not vehicles_part and "#" in line:
        # If # is at the start or no text before it, try to find vehicles in the string
        vehicles_part = line.split("#")[0].strip()
        
    return (first + vehicles_part).strip()

def find_incident_number(line):
    start = line.find("#")
    if start == -1:
        return ""
    # Incident number is usually 5-6 digits after #
    return find_between(line, "#", start + 7).strip()

def find_alarm_type(line):
    if ":" in line:
        end = findnth(line, ":", 0)
        # If it's a timestamp like 12:34:56, the character before : might be a digit
        if end > 1 and line[end-2:end-1].isdigit():
            return find_between(line, 0, end-3).strip()
    return line.rstrip()

def find_time(line):
    start = findnth(line, ":", 0)
    if start == -1:
        return ""
    temp = line[start-2:start-1]
    if temp.isdigit():
        return find_between(line, findnth(line, ":", 0)-3, findnth(line, ":", 1)+3).strip()
    return ""

def find_id(line):
    if "ID:" in line:
        return find_between(line, "ID:", len(line)).strip()
    return ""

def find_pager_address(line):
    return find_between(line, "Address:", "Function:").strip()

def process(line):
    page = {
        "type": "UNKNOWN",
        "page_id": "",
        "time": "",
        "call_type": "",
        "incident_number": "",
        "vehicles": "",
        "address": "",
        "intersection": "",
        "pager_address": "",
        "latitude": 0,
        "longitude": 0,
        "raw_text": ""
    }

    if "Alpha:" in line:
        is_eot = "<EOT>" in line
        clean_line = line.replace("<EOT>", "").replace("<NUL>", "").replace("\n", "")
        # Remove multimon-ng prefix for logic
        # POCSAG512: Address:  930032  Function: 2  Alpha:   ID:gdsJ3D...
        
        if "#" in clean_line and "Gd:" in clean_line:
            parts = clean_line.split("<LF>")
            page["type"] = "PAGE"
            page["page_id"] = find_id(parts[0])
            page["pager_address"] = find_pager_address(parts[0])

            # Find time
            found_time = ""
            for i in range(1, min(4, len(parts))):
                if ":" in parts[i]:
                    found_time = find_time(parts[i])
                    if found_time:
                        break
            page["time"] = found_time

            # Call type
            page["call_type"] = find_alarm_type(parts[1])

            # Incident number
            for p in parts:
                if "#" in p:
                    page["incident_number"] = find_incident_number(p)
                    break

            # Vehicles
            if len(parts) >= 3:
                page["vehicles"] = find_vehicles(parts[-2], parts[-3])

            # Address
            store = ""
            found_addr = False
            next_line_after_time = False
            for p in parts:
                if "Dt:" in p or "Dv:" in p:
                    page["address"] = find_address(p)
                    found_addr = True
                    break
                if next_line_after_time:
                    store += p + " "
                if page["time"] and page["time"] in p:
                    next_line_after_time = True
            
            page["address"] = (store + page["address"]).strip().replace("/", " at ")

            # Coordinates
            coords = get_long_lat(page["address"])
            page["latitude"] = coords["latitude"]
            page["longitude"] = coords["longitude"]

            # Intersection
            gd_index = -1
            for i, p in enumerate(parts):
                if "Gd:" in p:
                    gd_index = i
                    break
            
            if gd_index != -1:
                intersection_parts = []
                start_search = gd_index + 1
                # Find where vehicles line starts
                end_search = len(parts) - 2
                for i in range(start_search, end_search):
                    intersection_parts.append(parts[i])
                page["intersection"] = " ".join(intersection_parts).strip()

            # Swap intersection and address logic
            intersection_strings = [" & ", " at "]
            if any(x in page["address"] for x in intersection_strings):
                tmp = page["address"]
                page["address"] = page["intersection"]
                page["intersection"] = tmp
            
            page["raw_text"] = clean_line.replace("<LF>", "\n")
            return page

        else:
            parts = clean_line.split("<LF>")
            page_id = find_id(parts[0])
            if "ID:" not in parts[0]:
                 return {"type": "MSG", "page_id": page_id, "msg": " ".join(parts)}
            
            page["type"] = "MSG"
            page["page_id"] = page_id
            page["pager_address"] = find_pager_address(parts[0])
            page["raw_text"] = " ".join(parts[1:]).strip()
            return page

    elif "Address:" in line:
        # Address only message
        addr_match = re.search(r"Address:\s+(\d+)", line)
        if addr_match:
            return {"type": "ADDR", "addr": addr_match.group(1)}
            
    return {"type": "UNKNOWN", "raw": line}
