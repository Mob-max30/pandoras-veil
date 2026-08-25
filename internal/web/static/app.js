// ==========================================================================
// PANDORA'S VEIL | WhatsApp Web Dark Theme Logic & Crypto Bridge
// ==========================================================================

const state = {
    myHandle: 'PV-UJWAL',
    myFingerprint: 'BA64-5843',
    myPublicKey: 'age1q8ulqk4630rwqwavdst4fegn9st2zmrqdczvrx4uec9cmu6ah55swres5x',
    activeTarget: 'PV-PRANAV',
    activeTargetFP: '1E42-2834',
    isGroup: false,
    groupMembers: ['PV-PRANAV', 'PV-ALICE', 'PV-BOB'],
    ttl: 300,
    burnAfterReading: true,
    eventSource: null,
    conversations: {
        'PV-PRANAV': [
            {
                sender: 'PV-PRANAV',
                text: 'Hi! Live end-to-end encrypted session is active on Pandora\'s Veil.',
                timestamp: '20:30',
                isOutgoing: false
            },
            {
                sender: 'PV-PRANAV',
                text: 'why not fine?',
                timestamp: '20:32',
                isOutgoing: false
            }
        ],
        'Development': [
            {
                sender: 'PV-ALICE',
                text: 'Deployment complete for v1.2.5 on Render relay.',
                timestamp: 'Yesterday',
                isOutgoing: false
            },
            {
                sender: 'PV-UJWAL',
                text: 'Confirmed. Zero-knowledge live stream verified.',
                timestamp: 'Yesterday',
                isOutgoing: true
            }
        ],
        'PV-ALICE': [
            {
                sender: 'PV-ALICE',
                text: 'Hello Ujwal! Cryptographic key verified on relay.',
                timestamp: 'Yesterday',
                isOutgoing: false
            }
        ],
        'PV-BOB': [
            {
                sender: 'PV-BOB',
                text: 'Connected. Device fingerprint 915E-B66D confirmed.',
                timestamp: '22/08/2026',
                isOutgoing: false
            }
        ]
    }
};

function getAuthToken() {
    const meta = document.querySelector('meta[name="pandora-token"]');
    return meta ? meta.getAttribute('content') : '';
}

// DOM Elements
const myHandleEl = document.getElementById('my-handle');
const myFingerprintEl = document.getElementById('my-fingerprint');
const myAvatarEl = document.getElementById('my-avatar');
const activeChatAvatarEl = document.getElementById('active-chat-avatar');
const activeContactNameEl = document.getElementById('active-contact-name');
const activeContactFpEl = document.getElementById('active-contact-fp');
const activeTtlLabelEl = document.getElementById('active-ttl-label');
const chatMessagesContainerEl = document.getElementById('chat-messages-container');
const chatMessagesScrollEl = document.getElementById('chat-messages-scroll');
const chatInputEl = document.getElementById('chat-input');
const messageFormEl = document.getElementById('message-form');
const contactInfoDrawerEl = document.getElementById('contact-info-drawer');
const drawerAvatarEl = document.getElementById('drawer-avatar');
const drawerHandleEl = document.getElementById('drawer-handle');
const drawerFpEl = document.getElementById('drawer-fp');
const drawerPubkeyEl = document.getElementById('drawer-pubkey');
const drawerTtlSelectEl = document.getElementById('drawer-ttl-select');
const drawerBurnSwitchEl = document.getElementById('drawer-burn-switch');
const modalBackdropEl = document.getElementById('modal-backdrop');
const modalTitleEl = document.getElementById('modal-title');
const modalBodyEl = document.getElementById('modal-body');
const chatSearchEl = document.getElementById('chat-search');

// 1. Initialize App & Connect to Stream
async function initApp() {
    try {
        const token = getAuthToken();
        const res = await fetch('/api/identity', {
            headers: { 'X-Pandora-Token': token }
        });
        if (res.ok) {
            const data = await res.json();
            state.myHandle = data.handle || state.myHandle;
            state.myFingerprint = data.fingerprint || state.myFingerprint;
            state.myPublicKey = data.publicKey || state.myPublicKey;

            myHandleEl.textContent = state.myHandle;
            myFingerprintEl.textContent = `FP: ${state.myFingerprint}`;
            myAvatarEl.textContent = state.myHandle.replace('PV-', '').charAt(0) || 'U';
        }
    } catch (err) {
        console.warn('Failed to load local identity:', err);
    }

    setupEventListeners();
    connectSSEStream();
    renderActiveConversation();
}

// 2. Server-Sent Events Stream Bridge (Zero-Knowledge: Go decrypts before passing to JS)
function connectSSEStream() {
    if (state.eventSource) {
        state.eventSource.close();
    }

    const token = getAuthToken();
    const streamURL = token ? `/api/stream?token=${encodeURIComponent(token)}` : '/api/stream';
    state.eventSource = new EventSource(streamURL);

    state.eventSource.onmessage = (event) => {
        try {
            const data = JSON.parse(event.data);
            if (data && data.text) {
                const senderName = data.sender || state.activeTarget;
                const msg = {
                    sender: senderName,
                    text: data.text,
                    timestamp: data.timestamp || formatTime(new Date()),
                    isOutgoing: false
                };

                if (!state.conversations[senderName]) {
                    state.conversations[senderName] = [];
                }
                state.conversations[senderName].push(msg);

                // Update preview in sidebar
                const previewEl = document.getElementById(`preview-${senderName}`);
                const timeEl = document.getElementById(`chat-time-${senderName}`);
                if (previewEl) previewEl.textContent = data.text;
                if (timeEl) timeEl.textContent = msg.timestamp;

                if (senderName === state.activeTarget || (state.isGroup && senderName !== state.myHandle)) {
                    appendBubble(msg);
                }
            }
        } catch (err) {
            console.error('Error parsing stream event:', err);
        }
    };
}

// 3. Render Active Conversation
function renderActiveConversation() {
    const bannerHTML = `
        <div class="system-security-card">
            <svg viewBox="0 0 24 24" width="14" height="14" fill="#ffd279"><path d="M18 8h-1V6c0-2.76-2.24-5-5-5S7 3.24 7 6v2H6c-1.1 0-2 .9-2 2v10c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V10c0-1.1-.9-2-2-2zm-6 9c-1.1 0-2-.9-2-2s.9-2 2-2 2 .9 2 2-.9 2-2 2zm3.1-9H8.9V6c0-1.71 1.39-3.1 3.1-3.1 1.71 0 3.1 1.39 3.1 3.1v2z"></path></svg>
            <span>Messages are end-to-end encrypted with native device keys. No one outside of this chat can read them.</span>
        </div>
        <div class="date-divider"><span>TODAY</span></div>
    `;

    chatMessagesContainerEl.innerHTML = bannerHTML;

    const msgs = state.conversations[state.activeTarget] || [];
    msgs.forEach(msg => appendBubble(msg));

    scrollChatToBottom();
}

// 4. WhatsApp Message Bubble Renderer
function appendBubble(msg) {
    const bubble = document.createElement('div');
    bubble.className = `wa-bubble ${msg.isOutgoing ? 'outgoing' : 'incoming'}`;

    if (!msg.isOutgoing && state.isGroup) {
        const senderEl = document.createElement('div');
        senderEl.className = 'bubble-sender';
        senderEl.textContent = msg.sender;
        bubble.appendChild(senderEl);
    }

    const textEl = document.createElement('div');
    textEl.className = 'bubble-text';
    textEl.textContent = msg.text;
    bubble.appendChild(textEl);

    const metaEl = document.createElement('div');
    metaEl.className = 'bubble-meta';

    const timeSpan = document.createElement('span');
    timeSpan.className = 'bubble-time';
    timeSpan.textContent = msg.timestamp || formatTime(new Date());
    metaEl.appendChild(timeSpan);

    if (msg.isOutgoing) {
        const checkmarks = document.createElement('span');
        checkmarks.className = 'check-marks';
        checkmarks.innerHTML = `<svg viewBox="0 0 16 11" width="14" height="11" fill="#53bdeb" class="check-svg" style="display:inline-block; vertical-align:middle; margin-left:4px;"><path d="M11.07 1.48L5.78 6.77l-1.92-1.92-1.06 1.06 2.98 2.98 6.35-6.35-1.06-1.06zm3.86 0L8.58 7.83l.79.79 6.62-6.62-1.06-1.06z"></path></svg>`;
        metaEl.appendChild(checkmarks);
    }

    bubble.appendChild(metaEl);
    chatMessagesContainerEl.appendChild(bubble);
    scrollChatToBottom();
}

function scrollChatToBottom() {
    chatMessagesScrollEl.scrollTop = chatMessagesScrollEl.scrollHeight;
}

// 5. Send Message Handler (Delegates to native Go encryption)
async function handleSendMessage(e) {
    e.preventDefault();
    const text = chatInputEl.value.trim();
    if (!text) return;

    chatInputEl.value = '';

    const timestamp = formatTime(new Date());
    const msg = {
        sender: state.myHandle,
        text: text,
        timestamp: timestamp,
        isOutgoing: true
    };

    if (!state.conversations[state.activeTarget]) {
        state.conversations[state.activeTarget] = [];
    }
    state.conversations[state.activeTarget].push(msg);
    appendBubble(msg);

    // Update sidebar preview
    const previewEl = document.getElementById(`preview-${state.activeTarget}`);
    const timeEl = document.getElementById(`chat-time-${state.activeTarget}`);
    if (previewEl) previewEl.textContent = `You: ${text}`;
    if (timeEl) timeEl.textContent = timestamp;

    try {
        const token = getAuthToken();
        const payload = {
            target: state.activeTarget,
            isGroup: state.isGroup,
            groupMembers: state.groupMembers,
            text: text,
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
            appendBubble({
                sender: 'SYSTEM',
                text: `[System Notice] Message delivery failed: ${errData.error || 'Relay error'}`,
                timestamp: formatTime(new Date()),
                isOutgoing: false
            });
        }
    } catch (err) {
        appendBubble({
            sender: 'SYSTEM',
            text: `[System Notice] Network error: ${err.message}`,
            timestamp: formatTime(new Date()),
            isOutgoing: false
        });
    }
}

// 6. Event Listeners & Chat Switching
function setupEventListeners() {
    messageFormEl.addEventListener('submit', handleSendMessage);

    // Chat Item Click Handler
    document.querySelectorAll('.chat-item').forEach(item => {
        item.addEventListener('click', () => {
            document.querySelectorAll('.chat-item').forEach(i => i.classList.remove('active'));
            item.classList.add('active');

            const handle = item.getAttribute('data-handle');
            const type = item.getAttribute('data-type');
            const fp = item.getAttribute('data-fp') || 'Group';

            state.activeTarget = handle;
            state.activeTargetFP = fp;
            state.isGroup = type === 'group';

            activeContactNameEl.textContent = state.isGroup ? `#${handle}` : handle;
            activeContactFpEl.textContent = fp;
            activeChatAvatarEl.textContent = state.isGroup ? '#' : handle.replace('PV-', '').charAt(0);

            // Update Drawer Info
            drawerHandleEl.textContent = state.isGroup ? `#${handle}` : handle;
            drawerAvatarEl.textContent = state.isGroup ? '#' : handle.replace('PV-', '').charAt(0);
            drawerFpEl.textContent = fp;

            renderActiveConversation();
            chatInputEl.focus();
        });
    });

    // Chat Search Filter
    chatSearchEl.addEventListener('input', (e) => {
        const query = e.target.value.toLowerCase();
        document.querySelectorAll('.chat-item').forEach(item => {
            const name = item.querySelector('.chat-name').textContent.toLowerCase();
            if (name.includes(query)) {
                item.style.display = 'flex';
            } else {
                item.style.display = 'none';
            }
        });
    });
}

// 7. Drawers & Modals
function toggleContactInfoDrawer() {
    contactInfoDrawerEl.classList.toggle('hidden');
}

function toggleProfileDrawer() {
    openModal('My Device Identity', `
        <div style="display:flex; flex-direction:column; gap:12px; font-size:0.9rem;">
            <div><strong style="color:var(--wa-text-secondary);">Handle:</strong> <span style="font-weight:600;">${state.myHandle}</span></div>
            <div><strong style="color:var(--wa-text-secondary);">Fingerprint:</strong> <span style="color:var(--wa-text-yellow); font-family:var(--font-mono); font-weight:600;">${state.myFingerprint}</span></div>
            <div><strong style="color:var(--wa-text-secondary);">Public Key:</strong> <div style="font-size:0.75rem; color:#8696a0; word-break:break-all; font-family:var(--font-mono); margin-top:4px;">${state.myPublicKey}</div></div>
            <div style="color:#00a884; font-size:0.8rem; margin-top:6px;">[Secured] Private key stored locally on disk with 0600 permissions.</div>
        </div>
    `);
}

function toggleSettingsDrawer() {
    toggleContactInfoDrawer();
}

function openNewChatModal() {
    openModal('Start New Encrypted Chat', `
        <div style="display:flex; flex-direction:column; gap:12px;">
            <p style="color:var(--wa-text-secondary); font-size:0.88rem;">Enter recipient handle or comma-separated handles for a group:</p>
            <input type="text" id="new-chat-handle-input" placeholder="e.g. PV-BOB or PV-BOB,PV-ALICE" style="width:100%; padding:10px 12px; background:#2a3942; border:1px solid #222d34; color:#fff; border-radius:6px; font-size:0.95rem; outline:none;">
            <div style="display:flex; justify-content:flex-end; gap:8px; margin-top:8px;">
                <button type="button" class="wa-btn" onclick="startNewChatFromModal()">Start Chat</button>
            </div>
        </div>
    `);
}

function startNewChatFromModal() {
    const input = document.getElementById('new-chat-handle-input');
    if (input && input.value.trim()) {
        const val = input.value.trim();
        state.activeTarget = val;
        state.isGroup = val.includes(',');
        activeContactNameEl.textContent = state.isGroup ? `Group (${val})` : val;
        activeContactFpEl.textContent = 'Verified Key';
        if (!state.conversations[val]) {
            state.conversations[val] = [];
        }
        closeModal();
        renderActiveConversation();
    }
}

function openSecretDepositModal() {
    openModal('Send Self-Destructing Secret (GETDEL)', `
        <div style="display:flex; flex-direction:column; gap:12px;">
            <p style="color:var(--wa-text-secondary); font-size:0.88rem;">Enter confidential secret (API keys, passwords) to deposit for <strong>${state.activeTarget}</strong>:</p>
            <textarea id="secret-deposit-textarea" rows="4" placeholder="Confidential payload..." style="width:100%; padding:10px 12px; background:#2a3942; border:1px solid #222d34; color:#fff; border-radius:6px; font-family:var(--font-mono); font-size:0.88rem; outline:none;"></textarea>
            <div style="display:flex; justify-content:space-between; align-items:center; margin-top:8px;">
                <span style="font-size:0.8rem; color:#ffd279;">Burn-After-Reading: ON (${state.ttl}s TTL)</span>
                <button type="button" class="wa-btn" onclick="submitSecretDepositFromModal()">Deposit Secret</button>
            </div>
        </div>
    `);
}

async function submitSecretDepositFromModal() {
    const textarea = document.getElementById('secret-deposit-textarea');
    if (textarea && textarea.value.trim()) {
        const secret = textarea.value.trim();
        closeModal();
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
                    secret: secret,
                    ttl: state.ttl
                })
            });

            if (res.ok) {
                const data = await res.json();
                appendBubble({
                    sender: state.myHandle,
                    text: `[Self-Destructing Deposit Created] ID: ${data.id} (Destroyed upon first read by recipient)`,
                    timestamp: formatTime(new Date()),
                    isOutgoing: true
                });
            } else {
                alert('Failed to deposit secret.');
            }
        } catch (err) {
            alert(`Deposit failed: ${err.message}`);
        }
    }
}

function openDisappearingModal() {
    openModal('Disappearing Messages Timer', `
        <div style="display:flex; flex-direction:column; gap:12px;">
            <p style="color:var(--wa-text-secondary); font-size:0.88rem;">Set message lifespan timer for messages sent to ${state.activeTarget}:</p>
            <div style="display:flex; flex-direction:column; gap:8px;">
                <label style="cursor:pointer; display:flex; align-items:center; gap:8px;"><input type="radio" name="ttl-opt" value="60" ${state.ttl === 60 ? 'checked' : ''}> 60 Seconds</label>
                <label style="cursor:pointer; display:flex; align-items:center; gap:8px;"><input type="radio" name="ttl-opt" value="300" ${state.ttl === 300 ? 'checked' : ''}> 5 Minutes (300s)</label>
                <label style="cursor:pointer; display:flex; align-items:center; gap:8px;"><input type="radio" name="ttl-opt" value="3600" ${state.ttl === 3600 ? 'checked' : ''}> 1 Hour</label>
                <label style="cursor:pointer; display:flex; align-items:center; gap:8px;"><input type="radio" name="ttl-opt" value="86400" ${state.ttl === 86400 ? 'checked' : ''}> 24 Hours</label>
            </div>
            <div style="display:flex; justify-content:flex-end; margin-top:8px;">
                <button type="button" class="wa-btn" onclick="applyTtlFromModal()">Apply Timer</button>
            </div>
        </div>
    `);
}

function applyTtlFromModal() {
    const selected = document.querySelector('input[name="ttl-opt"]:checked');
    if (selected) {
        updateTTLFromSelect(selected.value);
        closeModal();
    }
}

function updateTTLFromSelect(val) {
    state.ttl = parseInt(val, 10) || 300;
    activeTtlLabelEl.textContent = `${state.ttl}s TTL`;
    if (drawerTtlSelectEl) drawerTtlSelectEl.value = String(state.ttl);
}

function toggleBurnSetting(enabled) {
    state.burnAfterReading = enabled;
}

function openModal(title, htmlContent) {
    modalTitleEl.textContent = title;
    modalBodyEl.innerHTML = htmlContent;
    modalBackdropEl.classList.remove('hidden');
}

function closeModal() {
    modalBackdropEl.classList.add('hidden');
    chatInputEl.focus();
}

function handleBackdropClick(e) {
    if (e.target === modalBackdropEl) {
        closeModal();
    }
}

function formatTime(d) {
    const pad = (n) => String(n).padStart(2, '0');
    return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

// Start
document.addEventListener('DOMContentLoaded', initApp);
