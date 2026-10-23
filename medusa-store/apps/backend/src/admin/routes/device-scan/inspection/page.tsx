import { Navigate, useSearchParams } from "react-router-dom"

const DeviceInspectionPage = () => {
  const [params] = useSearchParams()
  const deviceId = params.get("device_id")
  return <Navigate to={`/device-scan/returned${deviceId ? `?device_id=${encodeURIComponent(deviceId)}` : ""}`} replace />
}

export default DeviceInspectionPage
