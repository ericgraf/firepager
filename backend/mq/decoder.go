package mq

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/firepager/backend/models"
)

var (
	pocsagRegex   = regexp.MustCompile(`POCSAG\d+: Address:\s+(\d+)\s+Function: (\d)\s+Alpha:\s+(.*)`)
	incidentRegex = regexp.MustCompile(`#(\d+)`)
	vehicleTypes  = []string{"PT", "TA", "WR", "SC", "LA", "DC", "RE", "SO", "P"}
)

func decodeMessage(m *models.Message) {
	line := m.RawText

	// Check for Address only messages
	if strings.Contains(line, "Address:") && !strings.Contains(line, "Alpha:") {
		m.Type = "ADDR"
		re := regexp.MustCompile(`Address:\s+(\d+)`)
		match := re.FindStringSubmatch(line)
		if len(match) > 1 {
			m.Source = match[1]
		}
		return
	}

	match := pocsagRegex.FindStringSubmatch(line)
	if len(match) < 4 {
		m.Type = "UNKNOWN"
		return
	}

	m.Source = match[1]
	// match[2] is function, can be used if needed
	rawPayload := match[3]

	// Basic Cleaning
	cleanLine := strings.ReplaceAll(rawPayload, "<EOT>", "")
	cleanLine = strings.ReplaceAll(cleanLine, "<NUL>", "")
	cleanLine = strings.ReplaceAll(cleanLine, "\n", "")

	if strings.Contains(cleanLine, "#") && strings.Contains(cleanLine, "Gd:") {
		m.Type = "CALL"
		parts := strings.Split(cleanLine, "<LF>")

		// Page ID
		if strings.Contains(parts[0], "ID:") {
			m.PageID = strings.TrimSpace(strings.Split(parts[0], "ID:")[1])
		}

		// Call Type
		if len(parts) > 1 {
			m.CallType = extractAlarmType(parts[1])
		}

		// Incident Number
		for _, p := range parts {
			if strings.Contains(p, "#") {
				m.IncidentNumber = extractIncidentNumber(p)
				break
			}
		}

		// Vehicles and Address logic
		// This is a complex mapping of the python logic
		m.Vehicles = extractVehicles(parts)
		m.Address, m.Intersection = extractLocations(parts)
		m.Station = strings.Join(extractStations(m.Vehicles), ",")

		m.Text = strings.ReplaceAll(rawPayload, "<LF>", "\n")
	} else {
		m.Type = "MESSAGE"
		parts := strings.Split(cleanLine, "<LF>")
		if strings.Contains(parts[0], "ID:") {
			m.PageID = strings.TrimSpace(strings.Split(parts[0], "ID:")[1])
		}
		m.Text = strings.Join(parts, " ")
	}
}

func extractAlarmType(line string) string {
	idx := strings.Index(line, ":")
	if idx != -1 && idx > 2 {
		// Check if it's a timestamp
		if line[idx-1] >= '0' && line[idx-1] <= '9' {
			return strings.TrimSpace(line[:idx-3])
		}
	}
	return strings.TrimSpace(line)
}

func extractIncidentNumber(line string) string {
	match := incidentRegex.FindStringSubmatch(line)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

func extractVehicles(parts []string) []string {
	if len(parts) < 2 {
		return nil
	}

	last := parts[len(parts)-2]
	var segments []string

	if len(parts) >= 3 {
		prev := parts[len(parts)-3]
		if isVehicleLine(prev) {
			segments = append(segments, prev)
		}
	}

	idx := strings.Index(last, "#")
	if idx != -1 {
		segments = append(segments, strings.TrimSpace(last[:idx]))
	} else {
		segments = append(segments, strings.TrimSpace(last))
	}

	return strings.Fields(strings.Join(segments, " "))
}

func extractStations(vehicles []string) []string {
	stations := []string{}
	seen := make(map[string]bool)

	for _, v := range vehicles {
		s := getStationFromVehicle(v)
		if s == "" || seen[s] {
			continue
		}
		stations = append(stations, s)
		seen[s] = true
	}

	return stations
}

func getStationFromVehicle(v string) string {
	v = strings.ToUpper(strings.TrimSpace(v))
	v = strings.TrimRightFunc(v, func(r rune) bool {
		return r < '0' || r > '9'
	})

	// Remove prefixes
	for _, vt := range vehicleTypes {
		if strings.HasPrefix(v, vt) {
			v = strings.TrimPrefix(v, vt)
			break
		}
	}

	if _, err := strconv.Atoi(v); err != nil {
		return ""
	}

	// Station codes are two digits when available.
	// E.g. 91 -> 91, 101 -> 10, 9 -> 9
	if len(v) >= 2 {
		return v[:2]
	}
	return v
}

func isVehicleLine(line string) bool {
	words := strings.Fields(line)
	for _, w := range words {
		if getStationFromVehicle(w) != "" {
			return true
		}
	}
	return false
}

func extractLocations(parts []string) (string, string) {
	address := ""
	intersection := ""

	var gdIdx int = -1
	for i, p := range parts {
		if strings.Contains(p, "Dt:") || strings.Contains(p, "Dv:") {
			idx := strings.Index(p, "Dt:")
			if idx == -1 {
				idx = strings.Index(p, "Dv:")
			}
			address = strings.TrimSpace(p[:idx])
		}
		if strings.Contains(p, "Gd:") {
			gdIdx = i
		}
	}

	if gdIdx != -1 {
		interParts := []string{}
		for i := gdIdx + 1; i < len(parts)-2; i++ {
			interParts = append(interParts, parts[i])
		}
		intersection = strings.Join(interParts, " ")
	}

	address = strings.ReplaceAll(address, "/", " at ")

	// Swap logic
	if strings.Contains(address, " & ") || strings.Contains(address, " at ") {
		return intersection, address
	}

	return address, intersection
}
