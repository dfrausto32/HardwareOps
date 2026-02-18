CREATE INDEX IF NOT EXISTS idx_devices_hardware_id
  ON devices ((metadata #>> '{hwops,identity,hardwareId}'));
