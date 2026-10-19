import { MapContainer, TileLayer, Marker, Popup, useMap } from 'react-leaflet';
import 'leaflet/dist/leaflet.css';
import L from 'leaflet';
import './components.css';

// Fix for default marker icon in Leaflet
import icon from 'leaflet/dist/images/marker-icon.png';
import iconShadow from 'leaflet/dist/images/marker-shadow.png';

let DefaultIcon = L.icon({
    iconUrl: icon,
    shadowUrl: iconShadow,
    iconSize: [25, 41],
    iconAnchor: [12, 41]
});

L.Marker.prototype.options.icon = DefaultIcon;

interface CallMapProps {
  lat: number;
  lng: number;
  address: string;
  callType: string;
  incidentNumber: string;
}

function RecenterMap({ lat, lng }: { lat: number, lng: number }) {
  const map = useMap();
  map.setView([lat, lng], 15);
  return null;
}

export function CallMap({ lat, lng, address, callType, incidentNumber }: CallMapProps) {
  // If lat/lng are 0, it means it's not geocoded yet.
  const hasCoordinates = lat !== 0 || lng !== 0;

  if (!hasCoordinates) {
    return (
      <div className="map-placeholder">
        <p>No location data available for this call yet.</p>
        <p className="small-text">{address}</p>
      </div>
    );
  }

  return (
    <div className="map-wrapper">
      <MapContainer center={[lat, lng]} zoom={15} scrollWheelZoom={false} style={{ height: '300px', width: '100%' }}>
        <TileLayer
          attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
          url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
        />
        <Marker position={[lat, lng]}>
          <Popup>
            <strong>{callType}</strong><br />
            {address}<br />
            #{incidentNumber}
          </Popup>
        </Marker>
        <RecenterMap lat={lat} lng={lng} />
      </MapContainer>
    </div>
  );
}
