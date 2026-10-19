import { MapContainer, Marker, Popup, TileLayer, useMap } from 'react-leaflet';
import L from 'leaflet';
import 'leaflet/dist/leaflet.css';
import './components.css';

import icon from 'leaflet/dist/images/marker-icon.png';
import iconShadow from 'leaflet/dist/images/marker-shadow.png';

let DefaultIcon = L.icon({
  iconUrl: icon,
  shadowUrl: iconShadow,
  iconSize: [25, 41],
  iconAnchor: [12, 41]
});

L.Marker.prototype.options.icon = DefaultIcon;

export interface DashboardCall {
  id: string;
  timestamp: string;
  call_type: string;
  incident_number: string;
  address: string;
  intersection: string;
  latitude: number;
  longitude: number;
  station: string;
  vehicles: string[];
}

interface DashboardCallMapProps {
  calls: DashboardCall[];
}

function formatVehicles(vehicles: string[] | string | null | undefined) {
  if (Array.isArray(vehicles)) {
    return vehicles.filter(Boolean).join(', ');
  }

  return vehicles || '';
}

function FitCallBounds({ calls }: DashboardCallMapProps) {
  const map = useMap();

  if (calls.length === 1) {
    map.setView([calls[0].latitude, calls[0].longitude], 14);
    return null;
  }

  const bounds = L.latLngBounds(calls.map((call) => [call.latitude, call.longitude] as [number, number]));
  map.fitBounds(bounds, { padding: [32, 32] });
  return null;
}

export function DashboardCallMap({ calls }: DashboardCallMapProps) {
  const geocodedCalls = calls.filter((call) => call.latitude !== 0 || call.longitude !== 0);

  if (geocodedCalls.length === 0) {
    return (
      <div className="map-placeholder dashboard-map-placeholder">
        <p>No mapped calls available yet.</p>
        <p className="small-text">Calls will appear here once they have location coordinates.</p>
      </div>
    );
  }

  return (
    <div className="map-wrapper dashboard-map-wrapper">
      <MapContainer
        center={[geocodedCalls[0].latitude, geocodedCalls[0].longitude]}
        zoom={12}
        scrollWheelZoom={false}
        style={{ height: '420px', width: '100%' }}
      >
        <TileLayer
          attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
          url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
        />
        {geocodedCalls.map((call) => (
          <Marker key={call.id} position={[call.latitude, call.longitude]}>
            <Popup>
              <strong>{call.call_type}</strong><br />
              #{call.incident_number || 'N/A'}<br />
              {call.address}<br />
              {call.intersection ? `${call.intersection}` : null}
              {call.station ? <><br />Station {call.station}</> : null}
              {formatVehicles(call.vehicles) ? <><br />{formatVehicles(call.vehicles)}</> : null}
            </Popup>
          </Marker>
        ))}
        <FitCallBounds calls={geocodedCalls} />
      </MapContainer>
    </div>
  );
}
