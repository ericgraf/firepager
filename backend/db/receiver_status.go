package db

import (
	"fmt"
	"time"

	"github.com/firepager/backend/models"
)

func SaveReceiverStatus(status *models.ReceiverStatus) error {
	if status == nil || status.ReceiverID == "" || status.ObservedAt.IsZero() {
		return fmt.Errorf("receiver_id and observed_at are required")
	}
	_, err := DB.Exec(`INSERT INTO receiver_status (
		receiver_id, host, device_identifier, channel_name, description, nominal_frequency_hz,
		configured_gain_db, configured_ppm,
		status, role, frequency_hz, gain_db, ppm, active_detector,
		decoded_messages_count, calibrations_accepted_count, last_calibration_utc, observed_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,NOW())
	ON CONFLICT (receiver_id) DO UPDATE SET
		host=EXCLUDED.host, device_identifier=EXCLUDED.device_identifier,
		channel_name=EXCLUDED.channel_name, description=EXCLUDED.description,
		nominal_frequency_hz=EXCLUDED.nominal_frequency_hz,
		configured_gain_db=EXCLUDED.configured_gain_db, configured_ppm=EXCLUDED.configured_ppm,
		status=EXCLUDED.status, role=EXCLUDED.role,
		frequency_hz=EXCLUDED.frequency_hz, gain_db=EXCLUDED.gain_db, ppm=EXCLUDED.ppm,
		active_detector=EXCLUDED.active_detector,
		decoded_messages_count=EXCLUDED.decoded_messages_count,
		calibrations_accepted_count=EXCLUDED.calibrations_accepted_count,
		last_calibration_utc=EXCLUDED.last_calibration_utc,
		observed_at=EXCLUDED.observed_at, updated_at=NOW()
	WHERE receiver_status.observed_at <= EXCLUDED.observed_at`,
		status.ReceiverID, status.Host, status.DeviceIdentifier, status.ChannelName,
		status.Description, status.NominalFrequencyHz, status.ConfiguredGainDB,
		status.ConfiguredPPM, status.Status, status.Role,
		status.FrequencyHz, status.GainDB, status.PPM, status.ActiveDetector,
		status.DecodedMessagesCount, status.CalibrationsAcceptedCount,
		status.LastCalibrationUTC, status.ObservedAt)
	return err
}

func GetReceiverStatuses() ([]models.ReceiverStatus, error) {
	rows, err := DB.Query(`SELECT receiver_id, host, device_identifier, channel_name, description,
		nominal_frequency_hz, configured_gain_db, configured_ppm, status, role, frequency_hz, gain_db, ppm,
		active_detector, decoded_messages_count, calibrations_accepted_count,
		last_calibration_utc, observed_at, updated_at
		FROM receiver_status ORDER BY receiver_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.ReceiverStatus, 0)
	for rows.Next() {
		var row models.ReceiverStatus
		var freq, gain *float64
		var ppm *int
		var calibrated *time.Time
		var nominal, configuredGain *float64
		var configuredPPM *int
		if err := rows.Scan(&row.ReceiverID, &row.Host, &row.DeviceIdentifier,
			&row.ChannelName, &row.Description, &nominal, &configuredGain, &configuredPPM,
			&row.Status, &row.Role, &freq, &gain, &ppm,
			&row.ActiveDetector, &row.DecodedMessagesCount, &row.CalibrationsAcceptedCount,
			&calibrated, &row.ObservedAt, &row.UpdatedAt); err != nil {
			return nil, err
		}
		row.NominalFrequencyHz, row.ConfiguredGainDB, row.ConfiguredPPM = nominal, configuredGain, configuredPPM
		row.FrequencyHz, row.GainDB, row.PPM, row.LastCalibrationUTC = freq, gain, ppm, calibrated
		out = append(out, row)
	}
	return out, rows.Err()
}
