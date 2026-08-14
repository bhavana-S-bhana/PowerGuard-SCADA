// SCADA Industrial Power Plant Control Center JavaScript Core

(function () {
    'use strict';

    // State Store
    const state = {
        tags: {},
        activeAlarmsCount: 0,
        wsConnected: false,
        wsReconnectTimer: null,
        audioEnabled: true,
        currentAlertAlarmId: null,
        currentSelectedTagId: null,
        simActive: true
    };

    // DOM Elements
    const elements = {
        wsIndicator: document.getElementById('ws-indicator'),
        wsStatusText: document.getElementById('ws-status-text'),
        audioToggleBtn: document.getElementById('audio-toggle-btn'),
        audioIcon: document.getElementById('audio-icon'),
        simCheckbox: document.getElementById('sim-toggle-checkbox'),
        
        kpiMw: document.getElementById('kpi-mw'),
        kpiFreq: document.getElementById('kpi-freq'),
        kpiAlarms: document.getElementById('kpi-alarms'),
        kpiUptime: document.getElementById('kpi-uptime'),
        navAlarmBadge: document.getElementById('nav-alarm-badge'),
        
        navTabs: document.querySelectorAll('.nav-tab'),
        tabPages: document.querySelectorAll('.tab-page'),
        
        tagsTbody: document.getElementById('tags-tbody'),
        tagSearchInput: document.getElementById('tag-search-input'),
        alarmsTbody: document.getElementById('alarms-tbody'),
        auditTbody: document.getElementById('audit-tbody'),
        refreshAlarmsBtn: document.getElementById('refresh-alarms-btn'),
        refreshAuditBtn: document.getElementById('refresh-audit-btn'),
        
        // Alert Modal
        alertModal: document.getElementById('alert-modal'),
        modalTagId: document.getElementById('modal-tag-id'),
        modalMessage: document.getElementById('modal-message'),
        closeAlertModalBtn: document.getElementById('close-alert-modal'),
        // Override Modal
        overrideModal: document.getElementById('override-modal'),
        overrideTagId: document.getElementById('override-tag-id'),
        overrideTagName: document.getElementById('override-tag-name'),
        overrideCurrState: document.getElementById('override-curr-state'),
        highRiskWarningRow: document.getElementById('high-risk-warning-row'),
        overrideTargetState: document.getElementById('override-target-state'),
        overrideOperator: document.getElementById('override-operator'),
        overrideRationale: document.getElementById('override-rationale'),
        submitOverrideBtn: document.getElementById('submit-override-btn'),
        cancelOverrideBtn: document.getElementById('cancel-override-btn'),
        closeOverrideModalBtn: document.getElementById('close-override-modal')
    };

    // Web Audio Synthesizer for Industrial Alarm Chime
    let audioCtx = null;

    function playAlarmChime(severity) {
        if (!state.audioEnabled) return;
        try {
            if (!audioCtx) {
                audioCtx = new (window.AudioContext || window.webkitAudioContext)();
            }
            if (audioCtx.state === 'suspended') {
                audioCtx.resume();
            }
            
            const osc = audioCtx.createOscillator();
            const gain = audioCtx.createGain();

            osc.type = severity === 'CRITICAL' ? 'sawtooth' : 'sine';
            osc.frequency.setValueAtTime(severity === 'CRITICAL' ? 880 : 660, audioCtx.currentTime);
            osc.frequency.exponentialRampToValueAtTime(440, audioCtx.currentTime + 0.4);

            gain.gain.setValueAtTime(0.3, audioCtx.currentTime);
            gain.gain.exponentialRampToValueAtTime(0.01, audioCtx.currentTime + 0.4);

            osc.connect(gain);
            gain.connect(audioCtx.destination);

            osc.start();
            osc.stop(audioCtx.currentTime + 0.4);
        } catch (e) {
            console.log('Audio playback prevented or unsupported:', e);
        }
    }

    // Initialize Application
    function init() {
        bindEvents();
        setupNavigation();
        connectWebSocket();
        fetchSystemStatus();
        fetchTags();
        fetchAlarmHistory();
        fetchAuditLogs();

        // 1-second system clock ticker
        setInterval(fetchSystemStatus, 3000);
    }

    function bindEvents() {
        // Audio Toggle
        elements.audioToggleBtn.addEventListener('click', () => {
            state.audioEnabled = !state.audioEnabled;
            elements.audioIcon.textContent = state.audioEnabled ? '🔊' : '🔇';
        });

        // Simulation Toggle
        elements.simCheckbox.addEventListener('change', (e) => {
            toggleSimulation(e.target.checked);
        });

        // Modal Action Buttons
        elements.closeAlertModalBtn.addEventListener('click', () => {
    elements.alertModal.close();
});

        elements.closeOverrideModalBtn.addEventListener('click', () => elements.overrideModal.close());
        elements.cancelOverrideBtn.addEventListener('click', () => elements.overrideModal.close());
        elements.submitOverrideBtn.addEventListener('click', submitControlOverride);

        // Refresh Buttons
        elements.refreshAlarmsBtn.addEventListener('click', fetchAlarmHistory);
        elements.refreshAuditBtn.addEventListener('click', fetchAuditLogs);

        // Search Input Filter
        elements.tagSearchInput.addEventListener('input', renderTagsTable);

        // Node Click Handlers on SLD Schematic
        document.querySelectorAll('.breaker-node, .valve-node, .safety-node').forEach(node => {
            node.addEventListener('click', () => {
                const tagId = node.getAttribute('data-tagid');
                if (tagId) openOverrideModal(tagId);
            });
        });
    }

    function setupNavigation() {
        elements.navTabs.forEach(tab => {
            tab.addEventListener('click', () => {
                const targetTab = tab.getAttribute('data-tab');

                elements.navTabs.forEach(t => t.classList.remove('active'));
                elements.tabPages.forEach(p => p.classList.remove('active'));

                tab.classList.add('active');
                document.getElementById(targetTab).classList.add('active');

                if (targetTab === 'inventory-view') fetchTags();
                if (targetTab === 'alarms-view') fetchAlarmHistory();
                if (targetTab === 'audit-view') fetchAuditLogs();
            });
        });
    }

    // WebSocket Real-Time Connection Engine
    function connectWebSocket() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws`;

        elements.wsStatusText.textContent = 'CONNECTING...';
        elements.wsIndicator.className = 'ws-indicator';

        const ws = new WebSocket(wsUrl);

        ws.onopen = () => {
            console.log('[SCADA WebSocket] Real-time telemetry link established.');
            state.wsConnected = true;
            elements.wsStatusText.textContent = 'CONNECTED (LIVE)';
            elements.wsIndicator.className = 'ws-indicator connected';

            if (state.wsReconnectTimer) {
                clearTimeout(state.wsReconnectTimer);
                state.wsReconnectTimer = null;
            }
        };

        ws.onmessage = (event) => {
            try {
                const payload = JSON.parse(event.data);
                handleWebSocketMessage(payload);
            } catch (err) {
                console.error('[SCADA WebSocket] Error parsing JSON payload:', err);
            }
        };

        ws.onclose = () => {
            state.wsConnected = false;
            elements.wsStatusText.textContent = 'DISCONNECTED (RETRYING)';
            elements.wsIndicator.className = 'ws-indicator disconnected';
            
            // Automatic Reconnection with backoff
            if (!state.wsReconnectTimer) {
                state.wsReconnectTimer = setTimeout(connectWebSocket, 3000);
            }
        };

        ws.onerror = (err) => {
            console.error('[SCADA WebSocket] Socket error:', err);
            ws.close();
        };
    }

    function handleWebSocketMessage(payload) {
        console.log('[SCADA WS Payload Received]:', payload);

        switch (payload.type) {
            case 'INITIAL_STATE':
                fetchTags();
                fetchSystemStatus();
                break;

            case 'BINARY_STATE_CHANGE':
                onBinaryStateChange(payload);
                break;

            case 'CONTROL_OVERRIDE':
                onControlOverrideEvent(payload);
                break;

            case 'ALARM_ACKNOWLEDGED':
                fetchSystemStatus();
                fetchAlarmHistory();
                break;

            case 'SIMULATION_STATUS':
                if (payload.data && typeof payload.data.active === 'boolean') {
                    elements.simCheckbox.checked = payload.data.active;
                }
                break;
        }
    }

    // Trigger Immediate Real-Time Alert Modal upon Binary State Change
    function onBinaryStateChange(payload) {
        const tagId = payload.tag_id;
        const prevState = payload.prev_state;
        const currState = payload.curr_state;
        const priority = payload.priority || 'WARNING';

        // Update local tag store
        if (state.tags[tagId]) {
            state.tags[tagId].state = currState;
        }

        // Update Single-Line Diagram UI node
        updateSldNode(tagId, currState);

        // Update Tag Table
        renderTagsTable();

        // Refresh System KPIs & Alarms
        fetchSystemStatus();
        fetchAlarmHistory();

        // Play Synthesized Audio Chime
        playAlarmChime(priority);

        // Pop Up High-Priority Alert Modal
        triggerAlertModal({
            alarmId: payload.alarm_id,
            tagId: tagId,
            tagName: payload.tag_name || tagId,
            prevState: prevState,
            currState: currState,
            priority: priority,
            timestamp: payload.timestamp ? new Date(payload.timestamp).toLocaleString() : new Date().toLocaleString(),
            message: payload.data ? payload.data.message : `Tag ${tagId} toggled state from ${prevState} -> ${currState}`
        });
    }

    function onControlOverrideEvent(payload) {
        const tagId = payload.tag_id;
        const currState = payload.curr_state;

        if (state.tags[tagId]) {
            state.tags[tagId].state = currState;
        }
        updateSldNode(tagId, currState);
        renderTagsTable();
        fetchAuditLogs();
        fetchSystemStatus();
    }

    function triggerAlertModal(alert) {
    const modal = document.getElementById('alert-modal');
    const tagId = document.getElementById('modal-tag-id');
    const message = document.getElementById('modal-message');

    if (!modal) {
        return;
    }

    // Set Tag Name / ID
    if (tagId) {
        tagId.textContent =
            alert.tagId ||
            alert.tagName ||
            alert.tag ||
            'UNKNOWN TAG';
    }

    // Set Reason / Alarm Message
    if (message) {
        message.textContent =
            alert.message ||
            alert.reason ||
            alert.alarmMessage ||
            'Telemetry alarm detected.';
    }

    // Open modal
    if (typeof modal.showModal === 'function') {
        if (!modal.open) {
            modal.showModal();
        }
    }
}

    function acknowledgeCurrentAlert() {
        if (!state.currentAlertAlarmId) {
            elements.alertModal.close();
            return;
        }

        fetch('/api/alarms/acknowledge', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                alarm_id: state.currentAlertAlarmId,
                operator: 'OPERATOR_DESK_1'
            })
        })
        .then(res => res.json())
        .then(data => {
            if (data.success) {
                elements.alertModal.close();
                fetchSystemStatus();
                fetchAlarmHistory();
            }
        })
        .catch(err => console.error('Error acknowledging alarm:', err));
    }

    // Single Line Schematic Node Visual Updates
    function updateSldNode(tagId, stateVal) {
        const node = document.getElementById(`node-${tagId}`);
        if (!node) return;

        const indicator = node.querySelector('.node-state-indicator');
        if (indicator) {
            if (stateVal === 1) {
                indicator.classList.add('active');
            } else {
                indicator.classList.remove('active');
            }
        }

        // Generator Rotor Animation Logic
        if (tagId === 'G1_CB') {
            const wire = document.getElementById('wire-g1');
            const rotor = document.querySelector('#unit-g1 .rotor-icon');
            if (stateVal === 1) {
                if (wire) wire.classList.add('wire-active');
                if (rotor) rotor.style.animationPlayState = 'running';
            } else {
                if (wire) wire.classList.remove('wire-active');
                if (rotor) rotor.style.animationPlayState = 'paused';
            }
        }
        if (tagId === 'G2_CB') {
            const wire = document.getElementById('wire-g2');
            const rotor = document.querySelector('#unit-g2 .rotor-icon');
            if (stateVal === 1) {
                if (wire) wire.classList.add('wire-active');
                if (rotor) rotor.style.animationPlayState = 'running';
            } else {
                if (wire) wire.classList.remove('wire-active');
                if (rotor) rotor.style.animationPlayState = 'paused';
            }
        }
    }

    // Manual Override Modal Dialog
    function openOverrideModal(tagId) {
        const tag = state.tags[tagId];
        if (!tag) return;

        state.currentSelectedTagId = tagId;

        elements.overrideTagId.textContent = tag.tag_id;
        elements.overrideTagName.textContent = tag.name;
        elements.overrideCurrState.textContent = `${tag.state} (${tag.state === 1 ? 'CLOSED / ACTIVE' : 'OPEN / INACTIVE'})`;

        elements.overrideTargetState.value = tag.state === 1 ? '0' : '1';

        if (tag.is_high_risk) {
            elements.highRiskWarningRow.style.display = 'block';
            elements.overrideRationale.setAttribute('required', 'required');
        } else {
            elements.highRiskWarningRow.style.display = 'none';
            elements.overrideRationale.removeAttribute('required');
        }

        if (typeof elements.overrideModal.showModal === 'function') {
            elements.overrideModal.showModal();
        }
    }

    function submitControlOverride(e) {
        e.preventDefault();
        const tagId = state.currentSelectedTagId;
        const targetState = parseInt(elements.overrideTargetState.value, 10);
        const operator = elements.overrideOperator.value || 'OPERATOR_DESK_1';
        const rationale = elements.overrideRationale.value;

        const tag = state.tags[tagId];
        if (tag && tag.is_high_risk && !rationale.trim()) {
            alert('Mandatory Requirement: Please provide an operational rationale for high-risk equipment override.');
            return;
        }

        fetch('/api/control/override', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                tag_id: tagId,
                target_state: targetState,
                operator: operator,
                rationale: rationale
            })
        })
        .then(res => res.json())
        .then(data => {
            if (data.error) {
                alert('Override Error: ' + data.error);
            } else {
                elements.overrideModal.close();
                fetchTags();
                fetchAuditLogs();
            }
        })
        .catch(err => {
            alert('Network Error during override transaction: ' + err);
        });
    }

    // REST API Data Fetchers
    function fetchSystemStatus() {
        fetch('/api/system/status')
            .then(res => res.json())
            .then(data => {
                if (data.total_output_mw !== undefined) {
                    elements.kpiMw.textContent = `${data.total_output_mw} MW`;
                }
                if (data.grid_frequency_hz) {
                    elements.kpiFreq.textContent = `${data.grid_frequency_hz.toFixed(2)} Hz`;
                }
                if (data.active_alarms !== undefined) {
                    elements.kpiAlarms.textContent = data.active_alarms;
                    elements.navAlarmBadge.textContent = data.active_alarms;
                }
                if (data.uptime_formatted) {
                    elements.kpiUptime.textContent = data.uptime_formatted;
                }
                if (data.simulation_active !== undefined) {
                    elements.simCheckbox.checked = data.simulation_active;
                }
            })
            .catch(err => console.error('Error fetching system status:', err));
    }

    function fetchTags() {
        fetch('/api/tags')
            .then(res => res.json())
            .then(data => {
                if (data.tags) {
                    data.tags.forEach(t => {
                        state.tags[t.tag_id] = t;
                        updateSldNode(t.tag_id, t.state);
                    });
                    renderTagsTable();
                }
            })
            .catch(err => console.error('Error fetching tags:', err));
    }

    function renderTagsTable() {
        const query = (elements.tagSearchInput.value || '').toLowerCase();
        elements.tagsTbody.innerHTML = '';

        Object.values(state.tags).forEach(tag => {
            if (query && !tag.tag_id.toLowerCase().includes(query) && !tag.name.toLowerCase().includes(query) && !tag.category.toLowerCase().includes(query)) {
                return;
            }

            const tr = document.createElement('tr');
            const updated = tag.updated_at ? new Date(tag.updated_at).toLocaleTimeString() : 'N/A';
            const isHighRisk = tag.is_high_risk ? '⚡ HIGH RISK' : 'NORMAL';

            tr.innerHTML = `
                <td style="font-family: var(--font-mono); font-weight:700; color: var(--color-cyan);">${tag.tag_id}</td>
                <td>${tag.name}</td>
                <td><span class="badge-severity ${tag.priority}">${tag.category}</span></td>
                <td><span class="badge-state ${tag.state === 1 ? 'active' : 'inactive'}">${tag.state} (${tag.state === 1 ? 'CLOSED' : 'OPEN'})</span></td>
                <td style="font-family: var(--font-mono);">${tag.target_state}</td>
                <td><span style="font-family: var(--font-mono); font-size:0.75rem; color:${tag.is_high_risk ? 'var(--color-amber)' : 'var(--color-text-muted)'};">${isHighRisk}</span></td>
                <td><span class="badge-severity ${tag.priority}">${tag.priority}</span></td>
                <td style="font-family: var(--font-mono);">${updated}</td>
                <td>
                    <button class="btn btn-secondary btn-sm" onclick="window.SCADA.openOverride('${tag.tag_id}')">CONTROL</button>
                </td>
            `;
            elements.tagsTbody.appendChild(tr);
        });
    }

    function fetchAlarmHistory() {
        fetch('/api/alarms/history?limit=50')
            .then(res => res.json())
            .then(data => {
                if (data.alarms) {
                    renderAlarmsTable(data.alarms);
                }
            })
            .catch(err => console.error('Error fetching alarm history:', err));
    }

    function renderAlarmsTable(alarms) {
        elements.alarmsTbody.innerHTML = '';
        alarms.forEach(a => {
            const tr = document.createElement('tr');
            const ts = new Date(a.timestamp).toLocaleString();
            const ackBtn = a.status === 'ACTIVE'
                ? `<button class="btn btn-danger btn-sm" onclick="window.SCADA.ackAlarm(${a.id})">ACKNOWLEDGE</button>`
                : `<span style="color:var(--color-green); font-family:var(--font-mono);">ACKNOWLEDGED</span>`;

            tr.innerHTML = `
                <td style="font-family: var(--font-mono); font-weight:700;">#${a.id}</td>
                <td style="font-family: var(--font-mono);">${ts}</td>
                <td style="font-family: var(--font-mono); color: var(--color-cyan);">${a.tag_id}</td>
                <td>${a.tag_name}</td>
                <td><span class="badge-severity ${a.severity}">${a.severity}</span></td>
                <td>${a.message}</td>
                <td style="font-family: var(--font-mono);">${a.prev_state} ➔ ${a.curr_state}</td>
                <td><span class="badge-state ${a.status === 'ACTIVE' ? 'inactive' : 'active'}">${a.status}</span></td>
                <td style="font-family: var(--font-mono);">${a.ack_by || '-'}</td>
                <td>${ackBtn}</td>
            `;
            elements.alarmsTbody.appendChild(tr);
        });
    }

    function fetchAuditLogs() {
        fetch('/api/audit?limit=50')
            .then(res => res.json())
            .then(data => {
                if (data.audit_logs) {
                    renderAuditTable(data.audit_logs);
                }
            })
            .catch(err => console.error('Error fetching audit logs:', err));
    }

    function renderAuditTable(logs) {
        elements.auditTbody.innerHTML = '';
        logs.forEach(l => {
            const tr = document.createElement('tr');
            const ts = new Date(l.timestamp).toLocaleString();
            tr.innerHTML = `
                <td style="font-family: var(--font-mono); font-weight:700;">#${l.id}</td>
                <td style="font-family: var(--font-mono);">${ts}</td>
                <td style="font-weight:700; color: var(--color-cyan);">${l.operator}</td>
                <td><span class="badge-severity ${l.action.includes('HIGH_RISK') ? 'CRITICAL' : 'WARNING'}">${l.action}</span></td>
                <td style="font-family: var(--font-mono);">${l.tag_id}</td>
                <td style="font-family: var(--font-mono);">${l.prev_value}</td>
                <td style="font-family: var(--font-mono);">${l.new_value}</td>
                <td>${l.rationale || 'N/A'}</td>
            `;
            elements.auditTbody.appendChild(tr);
        });
    }

    function toggleSimulation(active) {
        fetch('/api/simulation/toggle', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ active: active })
        })
        .then(res => res.json())
        .then(data => {
            console.log('[SCADA Simulation Toggle]:', data);
        })
        .catch(err => console.error('Simulation toggle error:', err));
    }

    // Expose Global Helper Methods for Inline Table Action Callbacks
    window.SCADA = {
        openOverride: (tagId) => openOverrideModal(tagId),
        ackAlarm: (alarmId) => {
            fetch('/api/alarms/acknowledge', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ alarm_id: alarmId, operator: 'OPERATOR_DESK_1' })
            })
            .then(res => res.json())
            .then(() => {
                fetchAlarmHistory();
                fetchSystemStatus();
            });
        }
    };

    // DOM Ready Kickoff
    document.addEventListener('DOMContentLoaded', init);

})();
