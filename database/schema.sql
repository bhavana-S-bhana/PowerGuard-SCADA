-- SCADA Database Schema Initialization

CREATE TABLE IF NOT EXISTS tags (
    tag_id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    category TEXT NOT NULL,
    unit TEXT DEFAULT '',
    state INTEGER NOT NULL DEFAULT 0,
    target_state INTEGER NOT NULL DEFAULT 0,
    is_high_risk INTEGER NOT NULL DEFAULT 0,
    priority TEXT NOT NULL DEFAULT 'WARNING',
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS alarms (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tag_id TEXT NOT NULL,
    tag_name TEXT NOT NULL,
    severity TEXT NOT NULL,
    message TEXT NOT NULL,
    prev_state INTEGER NOT NULL,
    curr_state INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'ACTIVE',
    ack_by TEXT DEFAULT NULL,
    ack_at DATETIME DEFAULT NULL,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    operator TEXT NOT NULL,
    action TEXT NOT NULL,
    tag_id TEXT NOT NULL,
    prev_value INTEGER NOT NULL,
    new_value INTEGER NOT NULL,
    rationale TEXT DEFAULT ''
);

-- Seed Initial Binary Telemetry Tags (Exactly 30 Tags)
INSERT OR IGNORE INTO tags (tag_id, name, category, unit, state, target_state, is_high_risk, priority) VALUES
-- Generator Breakers (1 - 3)
('G1_CB', 'Generator Unit 1 Breaker', 'GENERATOR', 'BINARY', 1, 1, 1, 'CRITICAL'),
('G2_CB', 'Generator Unit 2 Breaker', 'GENERATOR', 'BINARY', 1, 1, 1, 'CRITICAL'),
('G3_CB', 'Generator Unit 3 Reserve Breaker', 'GENERATOR', 'BINARY', 0, 0, 1, 'CRITICAL'),

-- Transformer Breakers (4 - 6)
('TR1_CB', 'Step-Up Transformer 1 Breaker', 'TRANSFORMER', 'BINARY', 1, 1, 1, 'CRITICAL'),
('TR2_CB', 'Step-Up Transformer 2 Breaker', 'TRANSFORMER', 'BINARY', 0, 0, 1, 'WARNING'),
('TR3_CB', 'Step-Up Transformer 3 Breaker', 'TRANSFORMER', 'BINARY', 0, 0, 1, 'WARNING'),

-- Substation & Switchyard Breakers (7 - 10)
('BUS_TIE_CB', 'Substation Bus Tie Breaker', 'SWITCHYARD', 'BINARY', 0, 0, 1, 'CRITICAL'),
('BUS_BAR_2_CB', 'Auxiliary Busbar 2 Breaker', 'SWITCHYARD', 'BINARY', 1, 1, 1, 'CRITICAL'),
('GRID_FEED_CB1', '400kV Grid Feeder 1 Breaker', 'SWITCHYARD', 'BINARY', 1, 1, 1, 'CRITICAL'),
('GRID_FEED_CB2', '400kV Grid Feeder 2 Breaker', 'SWITCHYARD', 'BINARY', 1, 1, 1, 'CRITICAL'),

-- Thermal & Process Valves (11 - 18)
('V_MAIN_STEAM', 'Turbine Main Steam Isolation Valve', 'VALVE', 'BINARY', 1, 1, 1, 'CRITICAL'),
('V_AUX_COOLING', 'Auxiliary Condenser Cooling Valve', 'VALVE', 'BINARY', 1, 1, 0, 'WARNING'),
('V_REHEAT_STEAM', 'Reheat Steam Inlet Valve', 'VALVE', 'BINARY', 1, 1, 1, 'CRITICAL'),
('V_DRAIN_VALVE', 'Steam Drum Emergency Drain Valve', 'VALVE', 'BINARY', 0, 0, 1, 'CRITICAL'),
('V_FUEL_GAS_ISOL', 'Main Fuel Gas Isolation Valve', 'VALVE', 'BINARY', 1, 1, 1, 'CRITICAL'),
('COOLING_PUMP_1', 'Primary Feedwater Cooling Pump', 'VALVE', 'BINARY', 1, 1, 0, 'WARNING'),
('COOLING_PUMP_2', 'Secondary Feedwater Cooling Pump', 'VALVE', 'BINARY', 1, 1, 0, 'WARNING'),
('LUBE_OIL_PUMP_1', 'Turbine Lube Oil Main Pump', 'VALVE', 'BINARY', 1, 1, 0, 'CRITICAL'),

-- Safety & Protection Relays (19 - 22)
('ESD_RELAY', 'Emergency Shutdown Trip Relay', 'SAFETY', 'BINARY', 0, 0, 1, 'CRITICAL'),
('GRID_FREQ_RELAY', 'Grid Under-Frequency Protection Relay', 'SAFETY', 'BINARY', 0, 0, 1, 'CRITICAL'),
('FIRE_SUPPRESSION_RELAY', 'Switchyard Fire Protection Relay', 'SAFETY', 'BINARY', 0, 0, 1, 'CRITICAL'),
('OVERSPEED_TRIP_RELAY', 'Steam Turbine Mechanical Overspeed Trip', 'SAFETY', 'BINARY', 0, 0, 1, 'CRITICAL'),

-- Temperature Telemetry Sensors (23 - 27)
('TEMP_BEARING_G1', 'Generator 1 Bearing Over-Temp Sensor', 'SENSOR', 'BINARY', 0, 0, 0, 'CRITICAL'),
('TEMP_BEARING_G2', 'Generator 2 Bearing Over-Temp Sensor', 'SENSOR', 'BINARY', 0, 0, 0, 'CRITICAL'),
('TEMP_STATOR_G1', 'Generator 1 Stator Winding Temp Relay', 'SENSOR', 'BINARY', 0, 0, 0, 'WARNING'),
('TEMP_TRANSFORMER_TR1', 'Transformer 1 Winding Temp Relay', 'SENSOR', 'BINARY', 0, 0, 0, 'CRITICAL'),
('TEMP_CONDENSER_EXHAUST', 'Steam Condenser Exhaust Over-Temp Sensor', 'SENSOR', 'BINARY', 0, 0, 0, 'WARNING'),

-- Pressure Telemetry Sensors (28 - 30)
('PRESS_STEAM_DRUM_HI', 'Boiler Steam Drum High Pressure Relay', 'SENSOR', 'BINARY', 0, 0, 1, 'CRITICAL'),
('PRESS_LUBE_OIL_LOW', 'Turbine Lube Oil Low Pressure Relay', 'SENSOR', 'BINARY', 0, 0, 1, 'CRITICAL'),
('PRESS_VACUUM_TRIP', 'Condenser Vacuum Trip Relay', 'SENSOR', 'BINARY', 0, 0, 1, 'CRITICAL');
