// Command reparse reapplies the current pager decoder to stored call messages.
// It is intended for controlled, one-time repair after a decoder correction.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/firepager/backend/models"
	"github.com/firepager/backend/mq"
	"github.com/lib/pq"
)

type record struct {
	id           string
	rawText      string
	callType     string
	incident     string
	pageID       string
	vehicles     []string
	stations     []string
	address      string
	intersection string
	dv           string
	section      string
	grid         string
	alias        string
	parsed       models.Message
}

func main() {
	apply := flag.Bool("apply", false, "write changes (default is a read-only preview)")
	details := flag.Bool("details", false, "show incident numbers and changed field names")
	since := flag.String("since", "", "only reparse records at or after this RFC3339 timestamp")
	flag.Parse()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL must be set")
	}

	var sinceTime *time.Time
	if *since != "" {
		parsed, err := time.Parse(time.RFC3339, *since)
		if err != nil {
			log.Fatalf("invalid -since timestamp: %v", err)
		}
		sinceTime = &parsed
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}

	query := `SELECT id, COALESCE(raw_text,''), COALESCE(call_type,''),
		COALESCE(incident_number,''), COALESCE(page_id,''), vehicles, stations,
		COALESCE(address,''), COALESCE(intersection,''), COALESCE(dv,''),
		COALESCE(sec,''), COALESCE(gd,''), COALESCE(alias,'')
		FROM messages WHERE type='CALL' AND raw_text IS NOT NULL`
	args := []any{}
	if sinceTime != nil {
		query += " AND timestamp >= $1"
		args = append(args, *sinceTime)
	}
	query += " ORDER BY timestamp, id"

	rows, err := db.Query(query, args...)
	if err != nil {
		log.Fatal(err)
	}
	var updates []record
	fieldCounts := make(map[string]int)
	for rows.Next() {
		var r record
		if err := rows.Scan(&r.id, &r.rawText, &r.callType, &r.incident, &r.pageID,
			pq.Array(&r.vehicles), pq.Array(&r.stations), &r.address, &r.intersection,
			&r.dv, &r.section, &r.grid, &r.alias); err != nil {
			rows.Close()
			log.Fatal(err)
		}
		r.parsed = models.Message{RawText: r.rawText}
		mq.DecodeMessage(&r.parsed)
		if r.parsed.Type == "CALL" {
			fields := changedFields(r)
			if len(fields) != 0 {
				updates = append(updates, r)
				for _, field := range fields {
					fieldCounts[field]++
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		log.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Scanned call records; %d would be updated.\n", len(updates))
	if *details {
		for _, r := range updates {
			fmt.Printf("incident=%s fields=%s\n", r.incident, strings.Join(changedFields(r), ","))
		}
	}
	if len(fieldCounts) != 0 {
		fields := make([]string, 0, len(fieldCounts))
		for field := range fieldCounts {
			fields = append(fields, field)
		}
		sort.Strings(fields)
		counts := make([]string, 0, len(fields))
		for _, field := range fields {
			counts = append(counts, fmt.Sprintf("%s=%d", field, fieldCounts[field]))
		}
		fmt.Printf("Fields to correct: %s\n", strings.Join(counts, ", "))
	}
	if !*apply {
		fmt.Println("Read-only preview. Re-run with -apply to commit these changes.")
		return
	}

	tx, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback()
	const updateSQL = `UPDATE messages SET type=$1, page_id=$2, call_type=$3,
		incident_number=$4, vehicles=$5, stations=$6, address=$7, intersection=$8,
		dv=$9, sec=$10, gd=$11, alias=$12 WHERE id=$13`
	for _, r := range updates {
		vehicles := r.parsed.Vehicles
		if vehicles == nil {
			vehicles = []string{}
		}
		stations := r.parsed.Stations
		if stations == nil {
			stations = []string{}
		}
		if _, err := tx.Exec(updateSQL, r.parsed.Type, r.parsed.PageID, r.parsed.CallType,
			r.parsed.IncidentNumber, pq.Array(vehicles), pq.Array(stations),
			r.parsed.Address, r.parsed.Intersection, r.parsed.DV, r.parsed.Section,
			r.parsed.Grid, r.parsed.Alias, r.id); err != nil {
			log.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Updated %d records; coordinates, timestamps, and notification history were preserved.\n", len(updates))
}

func changedFields(r record) []string {
	p := r.parsed
	var fields []string
	if r.callType != p.CallType {
		fields = append(fields, "call_type")
	}
	if r.incident != p.IncidentNumber {
		fields = append(fields, "incident_number")
	}
	if r.pageID != p.PageID {
		fields = append(fields, "page_id")
	}
	if !equal(r.vehicles, p.Vehicles) {
		fields = append(fields, "vehicles")
	}
	if !equal(r.stations, p.Stations) {
		fields = append(fields, "stations")
	}
	if r.address != p.Address {
		fields = append(fields, "address")
	}
	if r.intersection != p.Intersection {
		fields = append(fields, "intersection")
	}
	if r.dv != p.DV {
		fields = append(fields, "dv")
	}
	if r.section != p.Section {
		fields = append(fields, "section")
	}
	if r.grid != p.Grid {
		fields = append(fields, "grid")
	}
	if r.alias != p.Alias {
		fields = append(fields, "alias")
	}
	return fields
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return len(a) == 0 && len(b) == 0
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
