UPDATE server_alerts
SET threshold = CASE
      WHEN item = 'status' THEN 0
      WHEN item IN ('cpu_usage', 'memory_usage', 'disk_usage') THEN LEAST(GREATEST(threshold, 1), 100)
      WHEN item IN ('read_iops', 'write_iops', 'bandwidth') THEN LEAST(GREATEST(threshold, 1), 1000000)
      WHEN item = 'expiry_reminder' THEN LEAST(GREATEST(threshold, 1), 7)
      ELSE threshold
    END,
    for_duration = CASE
      WHEN item = 'expiry_reminder' THEN 0
      ELSE LEAST(GREATEST(for_duration, 1), 1440)
    END
WHERE item IN (
  'status', 'cpu_usage', 'memory_usage', 'disk_usage',
  'read_iops', 'write_iops', 'bandwidth', 'expiry_reminder'
);

UPDATE team_alerts
SET threshold = CASE
      WHEN item = 'status' THEN 0
      WHEN item IN ('cpu_usage', 'memory_usage', 'disk_usage') THEN LEAST(GREATEST(threshold, 1), 100)
      WHEN item IN ('read_iops', 'write_iops', 'bandwidth') THEN LEAST(GREATEST(threshold, 1), 1000000)
      WHEN item = 'expiry_reminder' THEN LEAST(GREATEST(threshold, 1), 7)
      ELSE threshold
    END,
    for_duration = CASE
      WHEN item = 'expiry_reminder' THEN 0
      ELSE LEAST(GREATEST(for_duration, 1), 1440)
    END
WHERE item IN (
  'status', 'cpu_usage', 'memory_usage', 'disk_usage',
  'read_iops', 'write_iops', 'bandwidth', 'expiry_reminder'
);

ALTER TABLE server_alerts
  ADD CONSTRAINT server_alerts_config_bounds CHECK (
    (item = 'status' AND threshold = 0 AND for_duration BETWEEN 1 AND 1440)
    OR (item IN ('cpu_usage', 'memory_usage', 'disk_usage') AND threshold BETWEEN 1 AND 100 AND for_duration BETWEEN 1 AND 1440)
    OR (item IN ('read_iops', 'write_iops', 'bandwidth') AND threshold BETWEEN 1 AND 1000000 AND for_duration BETWEEN 1 AND 1440)
    OR (item = 'expiry_reminder' AND threshold BETWEEN 1 AND 7 AND for_duration = 0)
    OR item NOT IN (
      'status', 'cpu_usage', 'memory_usage', 'disk_usage',
      'read_iops', 'write_iops', 'bandwidth', 'expiry_reminder'
    )
  );

ALTER TABLE team_alerts
  ADD CONSTRAINT team_alerts_config_bounds CHECK (
    (item = 'status' AND threshold = 0 AND for_duration BETWEEN 1 AND 1440)
    OR (item IN ('cpu_usage', 'memory_usage', 'disk_usage') AND threshold BETWEEN 1 AND 100 AND for_duration BETWEEN 1 AND 1440)
    OR (item IN ('read_iops', 'write_iops', 'bandwidth') AND threshold BETWEEN 1 AND 1000000 AND for_duration BETWEEN 1 AND 1440)
    OR (item = 'expiry_reminder' AND threshold BETWEEN 1 AND 7 AND for_duration = 0)
    OR item NOT IN (
      'status', 'cpu_usage', 'memory_usage', 'disk_usage',
      'read_iops', 'write_iops', 'bandwidth', 'expiry_reminder'
    )
  );
