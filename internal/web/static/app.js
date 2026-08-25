// ==========================================================================
// PANDORA'S VEIL | Cyberpunk Web Dashboard Logic
// ==========================================================================

const state = {
    myHandle: 'PV-LOCAL',
    myFingerprint: '',
    myPublicKey: '',
    activeTarget: 'PV-PRANAV',
    isGroup: false,
    groupMembers: ['PV-PRANAV', 'PV-ALICE'],
    ttl: 300,
    burnAfterReading: true,
    eventSource: null
};

function getAuthToken() {
    const meta = document.querySelector('meta[name="pandora-token"]');
    return meta ? meta.getAttribute('content') : '';
}

// DOM Elements
const myHandleEl = document.getElementById('my-handle');
const myFingerprintEl = document.getElementById('my-fingerprint');
const chatMessagesEl = document.getElementById('chat-messages');
const chatInputEl = document.getElementById('chat-input');
const messageFormEl = document.getElementById('message-form');
const inputChanTagEl = document.getElementById('input-chan-tag');
const burnSwitchEl = document.getElementById('burn-switch');
const burnStatusTextEl = document.getElementById('burn-status-text');
const relayStatusEl = document.getElementById('relay-status');
const metaCreatedEl = document.getElementById('meta-created');
const metaRecipientEl = document.getElementById('meta-recipient');
const modalBackdropEl = document.getElementById('modal-backdrop');
const modalTitleEl = document.getElementById('modal-title');
const modalBodyEl = document.getElementById('modal-body');

// 1. Initialize Local Identity & Connect to Stream
async function initApp() {
    try {
        const token = getAuthToken();
        const res = await fetch('/api/identity', {
            headers: { 'X-Pandora-Token': token }
        });
        if (res.ok) {
            const data = await res.json();
            state.myHandle = data.handle || 'PV-LOCAL';
            state.myFingerprint = data.fingerprint || 'BA64-5843';
            state.myPublicKey = data.publicKey || '';

            myHandleEl.textContent = state.myHandle;
            myFingerprintEl.textContent = state.myFingerprint;
            metaCreatedEl.textContent = `${state.myHandle} ..`;
            metaRecipientEl.textContent = `${state.activeTarget} ..`;
        }
    } catch (err) {
        console.warn('Identity fetch warning:', err);
    }

    setupEventListeners();
    connectSSEStream();
}

// 2. Server-Sent Events Stream Bridge
function connectSSEStream() {
    if (state.eventSource) {
        state.eventSource.close();
    }

    const token = getAuthToken();
    const streamURL = token ? `/api/stream?token=${encodeURIComponent(token)}` : '/api/stream';
    state.eventSource = new EventSource(streamURL);

    state.eventSource.onopen = () => {
        relayStatusEl.textContent = 'CLOUD RELAY: CONNECTED';
        relayStatusEl.style.color = 'var(--text-green)';
    };

    state.eventSource.onmessage = (event) => {
        try {
            const data = JSON.parse(event.data);
            if (data && data.text) {
                appendMessage({
                    sender: data.sender || state.activeTarget,
                    text: data.text,
                    timestamp: formatTime(new Date()),
                    isOutgoing: false
                });
            }
        } catch (err) {
            console.error('Error parsing SSE event:', err);
        }
    };

    state.eventSource.onerror = () => {
        relayStatusEl.textContent = 'CLOUD RELAY: RECONNECTING...';
        relayStatusEl.style.color = 'var(--text-yellow)';
    };
}

// 3. Message Rendering
function appendMessage({ sender, text, timestamp, isOutgoing }) {
    const bubble = document.createElement('div');
    bubble.className = `speech-bubble ${isOutgoing ? 'outgoing-bubble' : 'incoming-bubble'}`;

    const header = document.createElement('div');
    header.className = 'bubble-header';

    const timeSpan = document.createElement('span');
    timeSpan.className = 'bubble-time';
    timeSpan.textContent = `[${timestamp || formatTime(new Date())}]`;

    const senderSpan = document.createElement('span');
    senderSpan.className = `bubble-sender ${isOutgoing ? 'green-text' : 'cyan-text'}`;
    senderSpan.textContent = isOutgoing ? '[YOU]' : sender;

    header.appendChild(timeSpan);
    header.appendChild(senderSpan);

    const body = document.createElement('div');
    body.className = 'bubble-body';
    body.textContent = text;

    bubble.appendChild(header);
    bubble.appendChild(body);

    chatMessagesEl.appendChild(bubble);
    chatMessagesEl.scrollTop = chatMessagesEl.scrollHeight;
}

// 4. Send Message Handler
async function handleSendMessage(e) {
    e.preventDefault();
    const rawText = chatInputEl.value.trim();
    if (!rawText) return;

    chatInputEl.value = '';

    // Handle Slash Commands
    if (rawText.startsWith('/')) {
        handleSlashCommand(rawText);
        return;
    }

    const timestamp = formatTime(new Date());

    // Render outgoing bubble locally
    appendMessage({
        sender: state.myHandle,
        text: rawText,
        timestamp: timestamp,
        isOutgoing: true
    });

    try {
        const token = getAuthToken();
        const payload = {
            target: state.activeTarget,
            isGroup: state.isGroup,
            groupMembers: state.groupMembers,
            text: rawText,
            ttl: state.ttl,
            burn: state.burnAfterReading
        };

        const res = await fetch('/api/send', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'X-Pandora-Token': token
            },
            body: JSON.stringify(payload)
        });

        if (!res.ok) {
            const errData = await res.json().catch(() => ({}));
            appendMessage({
                sender: 'SYSTEM',
                text: `Delivery failed: ${errData.error || 'Relay error'}`,
                timestamp: formatTime(new Date()),
                isOutgoing: false
            });
        }
    } catch (err) {
        appendMessage({
            sender: 'SYSTEM',
            text: `Delivery failed: ${err.message}`,
            timestamp: formatTime(new Date()),
            isOutgoing: false
        });
    }
}

// 5. Slash Commands Processor
function handleSlashCommand(cmd) {
    const parts = cmd.split(' ');
    const main = parts[0].toLowerCase();

    if (main === '/help') {
        openHelp();
    } else if (main === '/ttl') {
        if (parts[1]) {
            const val = parts[1].replace('s', '');
            setTTLValue(parseInt(val, 10) || 300);
            appendSystemNotice(`TTL set to ${parts[1]}`);
        }
    } else if (main === '/burn') {
        toggleBurnSetting();
    } else if (main === '/clear') {
        chatMessagesEl.innerHTML = '';
        appendSystemNotice('Chat history cleared.');
    } else if (main === '/deposit') {
        const secret = parts.slice(1).join(' ');
        if (secret) {
            createSecretDeposit(secret);
        } else {
            quickDeposit();
        }
    } else {
        appendSystemNotice(`Unknown command '${main}'. Type /help for options.`);
    }
}

function appendSystemNotice(text) {
    appendMessage({
        sender: 'POLICY',
        text: text,
        timestamp: formatTime(new Date()),
        isOutgoing: false
    });
}

// 6. TTL Selector
function setTTLValue(seconds) {
    state.ttl = seconds;
    document.querySelectorAll('.ttl-btn').forEach(btn => {
        if (parseInt(btn.getAttribute('data-ttl'), 10) === seconds) {
            btn.classList.add('active');
        } else {
            btn.classList.remove('active');
        }
    });
}

// 7. Burn-After-Reading Switch
function toggleBurnSetting() {
    state.burnAfterReading = !state.burnAfterReading;
    burnSwitchEl.checked = state.burnAfterReading;
    burnStatusTextEl.textContent = state.burnAfterReading ? '*ON*' : '*OFF*';
    burnStatusTextEl.className = state.burnAfterReading ? 'green-text bold' : 'bold';
    appendSystemNotice(`Burn-After-Reading (GETDEL) is now ${state.burnAfterReading ? 'ENABLED' : 'DISABLED'}`);
}

// 8. Event Listeners & Shortcuts
function setupEventListeners() {
    messageFormEl.addEventListener('submit', handleSendMessage);

    // TTL Buttons
    document.querySelectorAll('.ttl-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            const val = parseInt(btn.getAttribute('data-ttl'), 10);
            setTTLValue(val);
            appendSystemNotice(`TTL Expiration updated: ${btn.textContent}`);
        });
    });

    // Burn switch
    burnSwitchEl.addEventListener('change', () => {
        state.burnAfterReading = burnSwitchEl.checked;
        burnStatusTextEl.textContent = state.burnAfterReading ? '*ON*' : '*OFF*';
        burnStatusTextEl.className = state.burnAfterReading ? 'green-text bold' : 'bold';
    });

    // Channel selection
    document.querySelectorAll('.channel-item').forEach(item => {
        item.addEventListener('click', () => {
            document.querySelectorAll('.channel-item').forEach(i => i.classList.remove('active'));
            item.classList.add('active');

            const type = item.getAttribute('data-type');
            const target = item.getAttribute('data-target');

            if (type === 'group') {
                state.isGroup = true;
                state.activeTarget = target;
                inputChanTagEl.textContent = `[ #${target} ]`;
                document.getElementById('active-mode-title').textContent = `#${target}`;
                metaRecipientEl.textContent = `Group: #${target}`;
            } else {
                state.isGroup = false;
                state.activeTarget = target;
                inputChanTagEl.textContent = `[ @${target} ]`;
                document.getElementById('active-mode-title').textContent = target;
                metaRecipientEl.textContent = `${target} ..`;
            }

            appendSystemNotice(`Switched channel to ${type === 'group' ? '#' : '@'}${target}`);
        });
    });

    // Keyboard Shortcuts
    window.addEventListener('keydown', (e) => {
        if (e.ctrlKey && e.key.toLowerCase() === 'n') {
            e.preventDefault();
            quickNewGroup();
        } else if (e.ctrlKey && e.key.toLowerCase() === 'k') {
            e.preventDefault();
            quickDeposit();
        } else if (e.ctrlKey && e.key.toLowerCase() === 'b') {
            e.preventDefault();
            toggleBurnSetting();
        } else if (e.key === 'Tab' && document.activeElement !== chatInputEl) {
            e.preventDefault();
            chatInputEl.focus();
        }
    });
}

// 9. Modals & Quick Actions
function openModal(title, contentHTML) {
    modalTitleEl.textContent = title;
    modalBodyEl.innerHTML = contentHTML;
    modalBackdropEl.classList.remove('hidden');
}

function closeModal() {
    modalBackdropEl.classList.add('hidden');
    chatInputEl.focus();
}

function quickNewGroup() {
    openModal('CREATE / JOIN GROUP CHAT', `
        <div style="display:flex; flex-direction:column; gap:10px;">
            <p style="color:var(--text-dim); font-size:0.85rem;">Enter comma-separated recipient handles to establish multi-recipient encrypted group:</p>
            <input type="text" id="group-handles-input" class="cyber-input" placeholder="e.g. PV-PRANAV,PV-ALICE,PV-BOB" style="width:100%; padding:8px 10px; background:#0e111a; border:1px solid var(--border-cyan); color:#fff; border-radius:6px; font-family:var(--font-mono);">
            <div style="display:flex; justify-content:flex-end; gap:8px; margin-top:8px;">
                <button type="button" class="send-action-btn" onclick="confirmNewGroup()">CONNECT</button>
            </div>
        </div>
    `);
}

function confirmNewGroup() {
    const input = document.getElementById('group-handles-input');
    if (input && input.value.trim()) {
        const handles = input.value.split(',').map(h => h.trim()).filter(Boolean);
        state.groupMembers = handles;
        state.isGroup = true;
        state.activeTarget = handles.join(',');
        inputChanTagEl.textContent = `[ Group: ${handles.length} Peers ]`;
        closeModal();
        appendSystemNotice(`Group session initialized targeting: ${handles.join(', ')}`);
    }
}

function quickDeposit() {
    openModal('CREATE SELF-DESTRUCTING SECRET DEPOSIT', `
        <div style="display:flex; flex-direction:column; gap:10px;">
            <p style="color:var(--text-dim); font-size:0.85rem;">Enter secret plaintext to deposit with Redis GETDEL policy:</p>
            <textarea id="deposit-secret-input" rows="4" style="width:100%; padding:8px 10px; background:#0e111a; border:1px solid var(--border-magenta); color:#fff; border-radius:6px; font-family:var(--font-mono);" placeholder="Paste API keys, passwords, or confidential payloads..."></textarea>
            <div style="display:flex; justify-content:space-between; align-items:center; margin-top:8px;">
                <span style="color:var(--text-green); font-size:0.8rem;">TTL: ${state.ttl}s | Burn: ON</span>
                <button type="button" class="send-action-btn" style="border-color:var(--border-magenta); color:var(--text-magenta);" onclick="confirmDeposit()">DEPOSIT</button>
            </div>
        </div>
    `);
}

async function confirmDeposit() {
    const input = document.getElementById('deposit-secret-input');
    if (input && input.value.trim()) {
        const text = input.value.trim();
        closeModal();
        await createSecretDeposit(text);
    }
}

async function createSecretDeposit(plaintext) {
    try {
        const token = getAuthToken();
        const res = await fetch('/api/deposit', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'X-Pandora-Token': token
            },
            body: JSON.stringify({
                recipient: state.activeTarget,
                secret: plaintext,
                ttl: state.ttl
            })
        });

        if (res.ok) {
            const data = await res.json();
            appendSystemNotice(`Secret Deposit Created! ID: ${data.id || 'pv_' + Date.now()} (Self-destructs on read)`);
        } else {
            appendSystemNotice('Failed to create secret deposit.');
        }
    } catch (err) {
        appendSystemNotice(`Deposit error: ${err.message}`);
    }
}

function openHelp() {
    openModal('COMMANDS & SHORTCUTS HELP', `
        <div style="display:flex; flex-direction:column; gap:8px; font-size:0.85rem;">
            <div><strong style="color:var(--text-cyan);">/help</strong> : Show this help menu</div>
            <div><strong style="color:var(--text-cyan);">/ttl &lt;60s|300s|1h|24h&gt;</strong> : Update TTL expiration</div>
            <div><strong style="color:var(--text-cyan);">/burn</strong> : Toggle Burn-After-Reading state</div>
            <div><strong style="color:var(--text-cyan);">/deposit &lt;text&gt;</strong> : Create encrypted self-destructing deposit</div>
            <div><strong style="color:var(--text-cyan);">/clear</strong> : Clear chat message pane</div>
            <hr style="border-color:var(--border-dim); margin:6px 0;">
            <div><strong style="color:var(--text-green);">[Ctrl+N]</strong> : New Group Session</div>
            <div><strong style="color:var(--text-green);">[Ctrl+K]</strong> : Create Secret Deposit</div>
            <div><strong style="color:var(--text-green);">[Ctrl+B]</strong> : Toggle Burn-After-Reading</div>
            <div><strong style="color:var(--text-green);">[Tab]</strong> : Switch focus to message input</div>
        </div>
    `);
}

function switchFocus() {
    chatInputEl.focus();
}

function formatTime(d) {
    const pad = (n) => String(n).padStart(2, '0');
    return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

// Start
document.addEventListener('DOMContentLoaded', initApp);
