// ==========================================================================
// PANDORA'S VEIL | Linen Aesthetic Logic & Zero-Knowledge Crypto Bridge
// ==========================================================================

const state = {
    myHandle: 'PV-UJWAL',
    myFingerprint: 'BA64-5843',
    myPublicKey: 'age1q8ulqk4630rwqwavdst4fegn9st2zmrqdczvrx4uec9cmu6ah55swres5x',
    activeTarget: 'PV-PRANAV',
    activeDisplayName: 'Amara Kessler',
    activeTargetFP: '1E42-2834',
    activeAvatar: 'AK',
    activeAvatarColor: 'avatar-sand',
    isGroup: false,
    groupMembers: ['PV-PRANAV', 'PV-ALICE', 'PV-BOB'],
    ttl: 300,
    burnAfterReading: true,
    eventSource: null,
    conversations: {
        'PV-PRANAV': [
            {
                sender: 'Amara Kessler',
                text: 'Morning — how\'s the highway dataset labeling coming along?',
                timestamp: '09:12',
                isOutgoing: false
            },
            {
                sender: 'PV-UJWAL',
                text: 'About 70% through. Frame extraction was slower than expected but accuracy\'s holding up well.',
                timestamp: '09:15',
                isOutgoing: true
            },
            {
                sender: 'Amara Kessler',
                text: 'Great — that lines up well with the review timeline.',
                timestamp: '09:20',
                isOutgoing: false
            },
            {
                sender: 'Amara Kessler',
                text: 'Can you send the revised deck before noon?',
                timestamp: '09:41',
                isOutgoing: false
            },
            {
                sender: 'PV-UJWAL',
                text: 'Yep, finishing the last two slides now. Will have it to you by 11:30.',
                timestamp: '09:44',
                isOutgoing: true
            }
        ],
        'PV-DEV': [
            {
                sender: 'Dev Ramanathan',
                text: 'Sounds good — see you at the studio.',
                timestamp: '08:57',
                isOutgoing: false
            }
        ],
        'Studio-Nine': [
            {
                sender: 'Priya',
                text: 'Invoices are attached below for Q3 milestone.',
                timestamp: 'Yesterday',
                isOutgoing: false
            }
        ],
        'PV-LEO': [
            {
                sender: 'Leo Ferreira',
                text: 'Ha — fair point, noted.',
                timestamp: 'Yesterday',
                isOutgoing: false
            }
        ],
        'PV-MIRA': [
            {
                sender: 'Mira Okafor',
                text: 'Encrypted voice note payload received.',
                timestamp: 'Monday',
                isOutgoing: false
            }
        ],
        'PV-TOMAS': [
            {
                sender: 'Tomas Weber',
                text: 'Booked the flights, confirmation below.',
                timestamp: 'Monday',
                isOutgoing: false
            }
        ],
        'Family': [
            {
                sender: 'Dad',
                text: 'Dinner\'s at 7 on Saturday',
                timestamp: 'Sunday',
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
const activeHeaderAvatarEl = document.getElementById('active-header-avatar');
const activeContactTitleEl = document.getElementById('active-contact-title');
const activeFpSubtleEl = document.getElementById('active-fp-subtle');
const chatMessagesContainerEl = document.getElementById('chat-messages-container');
const chatMessagesScrollEl = document.getElementById('chat-messages-scroll');
const chatInputEl = document.getElementById('chat-input');
const messageFormEl = document.getElementById('message-form');
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

// 2. Server-Sent Events Stream Bridge (Go handles decryption)
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
                const timestamp = data.timestamp || formatTime(new Date());
                const msg = {
                    sender: senderName,
                    text: data.text,
                    timestamp: timestamp,
                    isOutgoing: false
                };

                if (!state.conversations[senderName]) {
                    state.conversations[senderName] = [];
                }
                state.conversations[senderName].push(msg);

                const previewEl = document.getElementById(`preview-${senderName}`);
                const timeEl = document.getElementById(`time-${senderName}`);
                if (previewEl) previewEl.textContent = data.text;
                if (timeEl) timeEl.textContent = timestamp;

                if (senderName === state.activeTarget || (state.isGroup && senderName !== state.myHandle)) {
                    appendLinenBubble(msg);
                }
            }
        } catch (err) {
            console.error('Error parsing stream event:', err);
        }
    };
}

// 3. Render Active Conversation
function renderActiveConversation() {
    const separatorHTML = `
        <div class="linen-date-separator">
            <span class="sep-line"></span>
            <span class="sep-label">TODAY</span>
            <span class="sep-line"></span>
        </div>
    `;

    chatMessagesContainerEl.innerHTML = separatorHTML;

    const msgs = state.conversations[state.activeTarget] || [];
    msgs.forEach(msg => appendLinenBubble(msg));

    scrollChatToBottom();
}

// 4. Append Linen Message Card
function appendLinenBubble(msg) {
    const groupEl = document.createElement('div');
    groupEl.className = `linen-bubble-group ${msg.isOutgoing ? 'outgoing-group' : 'incoming-group'}`;

    const cardEl = document.createElement('div');
    cardEl.className = `linen-card ${msg.isOutgoing ? 'outgoing-card' : 'incoming-card'}`;

    const pEl = document.createElement('p');
    pEl.textContent = msg.text;
    cardEl.appendChild(pEl);
    groupEl.appendChild(cardEl);

    if (!msg.isOutgoing && msg.timestamp) {
        const timeSpan = document.createElement('span');
        timeSpan.className = 'bubble-time-stamp';
        timeSpan.textContent = msg.timestamp;
        groupEl.appendChild(timeSpan);
    }

    chatMessagesContainerEl.appendChild(groupEl);
    scrollChatToBottom();
}

function scrollChatToBottom() {
    chatMessagesScrollEl.scrollTop = chatMessagesScrollEl.scrollHeight;
}

// 5. Send Message (Native Go Encryption)
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
    appendLinenBubble(msg);

    const previewEl = document.getElementById(`preview-${state.activeTarget}`);
    const timeEl = document.getElementById(`time-${state.activeTarget}`);
    if (previewEl) previewEl.textContent = text;
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
            appendLinenBubble({
                sender: 'SYSTEM',
                text: `Delivery failed: ${errData.error || 'Relay error'}`,
                timestamp: formatTime(new Date()),
                isOutgoing: false
            });
        }
    } catch (err) {
        appendLinenBubble({
            sender: 'SYSTEM',
            text: `Network error: ${err.message}`,
            timestamp: formatTime(new Date()),
            isOutgoing: false
        });
    }
}

// 6. Setup Event Listeners & Contact Switching
function setupEventListeners() {
    messageFormEl.addEventListener('submit', handleSendMessage);

    // Chat Card Selection
    document.querySelectorAll('.chat-card').forEach(card => {
        card.addEventListener('click', () => {
            document.querySelectorAll('.chat-card').forEach(c => c.classList.remove('active'));
            card.classList.add('active');

            const handle = card.getAttribute('data-handle');
            const name = card.getAttribute('data-name');
            const fp = card.getAttribute('data-fp') || 'Group';
            const avatar = card.getAttribute('data-avatar') || name.charAt(0);
            const avatarColor = card.getAttribute('data-avatar-color') || 'avatar-sand';
            const type = card.getAttribute('data-type');

            state.activeTarget = handle;
            state.activeDisplayName = name;
            state.activeTargetFP = fp;
            state.activeAvatar = avatar;
            state.activeAvatarColor = avatarColor;
            state.isGroup = type === 'group';

            activeContactTitleEl.textContent = name;
            activeHeaderAvatarEl.textContent = avatar;
            activeHeaderAvatarEl.className = `avatar ${avatarColor}`;
            activeFpSubtleEl.textContent = `• ${fp}`;

            renderActiveConversation();
            chatInputEl.focus();
        });
    });

    // Chat Search Filter
    chatSearchEl.addEventListener('input', (e) => {
        const query = e.target.value.toLowerCase();
        document.querySelectorAll('.chat-card').forEach(card => {
            const name = card.querySelector('.contact-card-name').textContent.toLowerCase();
            if (name.includes(query)) {
                card.style.display = 'flex';
            } else {
                card.style.display = 'none';
            }
        });
    });
}

// 7. Modals
function toggleProfileModal() {
    openModal('Device Security Credentials', `
        <div style="display:flex; flex-direction:column; gap:16px; font-size:0.92rem; color:var(--linen-text-dark);">
            <div><strong style="color:#636765;">Handle:</strong> <span style="font-weight:600;">${state.myHandle}</span></div>
            <div><strong style="color:#636765;">Fingerprint:</strong> <span style="color:var(--linen-card-green); font-family:var(--font-mono); font-weight:600;">${state.myFingerprint}</span></div>
            <div><strong style="color:#636765;">Public Key:</strong> <div style="font-size:0.75rem; color:#787b78; word-break:break-all; font-family:var(--font-mono); margin-top:6px; background:#fff; padding:10px; border-radius:8px;">${state.myPublicKey}</div></div>
            <div style="color:var(--linen-card-green); font-size:0.82rem; margin-top:4px;">🔒 Cryptographic keypair secured on physical device (0600 permissions).</div>
        </div>
    `);
}

function openContactDetailsModal() {
    openModal(`${state.activeDisplayName}`, `
        <div style="display:flex; flex-direction:column; gap:16px; font-size:0.92rem; color:var(--linen-text-dark);">
            <div><strong style="color:#636765;">Handle:</strong> <span>${state.activeTarget}</span></div>
            <div><strong style="color:#636765;">Device Fingerprint:</strong> <span style="font-family:var(--font-mono); color:var(--linen-card-green); font-weight:600;">${state.activeTargetFP}</span></div>
            <div><strong style="color:#636765;">Encryption Suite:</strong> <span>age / X25519 Native Go Bridge</span></div>
            <div><strong style="color:#636765;">Relay Status:</strong> <span style="color:var(--linen-online); font-weight:600;">Connected</span></div>
        </div>
    `);
}

function openNewChatModal() {
    openModal('Start Correspondence', `
        <div style="display:flex; flex-direction:column; gap:14px;">
            <p style="color:#636765; font-size:0.88rem;">Enter recipient handle or comma-separated handles for a group:</p>
            <input type="text" id="new-chat-handle-input" placeholder="e.g. PV-BOB or PV-BOB,PV-ALICE" style="width:100%; padding:12px 14px; background:#fff; border:1px solid #dfd8cc; color:#222725; border-radius:10px; font-size:0.95rem; outline:none;">
            <div style="display:flex; justify-content:flex-end; gap:8px; margin-top:8px;">
                <button type="button" class="linen-btn" onclick="startNewChatFromModal()">Start Correspondence</button>
            </div>
        </div>
    `);
}

function startNewChatFromModal() {
    const input = document.getElementById('new-chat-handle-input');
    if (input && input.value.trim()) {
        const val = input.value.trim();
        state.activeTarget = val;
        state.activeDisplayName = val;
        state.isGroup = val.includes(',');
        activeContactTitleEl.textContent = val;
        activeFpSubtleEl.textContent = '• Verified Key';
        if (!state.conversations[val]) {
            state.conversations[val] = [];
        }
        closeModal();
        renderActiveConversation();
    }
}

function openSecretDepositModal() {
    openModal('Send Confidential Deposit (GETDEL)', `
        <div style="display:flex; flex-direction:column; gap:14px;">
            <p style="color:#636765; font-size:0.88rem;">Enter confidential secret (API keys, recovery codes) to deposit for <strong>${state.activeDisplayName}</strong>:</p>
            <textarea id="secret-deposit-textarea" rows="4" placeholder="Confidential payload..." style="width:100%; padding:12px 14px; background:#fff; border:1px solid #dfd8cc; color:#222725; border-radius:10px; font-family:var(--font-mono); font-size:0.88rem; outline:none;"></textarea>
            <div style="display:flex; justify-content:space-between; align-items:center; margin-top:8px;">
                <span style="font-size:0.8rem; color:var(--linen-terracotta);">Burn-After-Reading: ON (${state.ttl}s TTL)</span>
                <button type="button" class="linen-btn" onclick="submitSecretDepositFromModal()">Deposit Secret</button>
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
                appendLinenBubble({
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
        <div style="display:flex; flex-direction:column; gap:14px;">
            <p style="color:#636765; font-size:0.88rem;">Set message lifespan timer for messages sent to ${state.activeDisplayName}:</p>
            <div style="display:flex; flex-direction:column; gap:10px;">
                <label style="cursor:pointer; display:flex; align-items:center; gap:8px;"><input type="radio" name="ttl-opt" value="60" ${state.ttl === 60 ? 'checked' : ''}> 60 Seconds</label>
                <label style="cursor:pointer; display:flex; align-items:center; gap:8px;"><input type="radio" name="ttl-opt" value="300" ${state.ttl === 300 ? 'checked' : ''}> 5 Minutes (300s)</label>
                <label style="cursor:pointer; display:flex; align-items:center; gap:8px;"><input type="radio" name="ttl-opt" value="3600" ${state.ttl === 3600 ? 'checked' : ''}> 1 Hour</label>
                <label style="cursor:pointer; display:flex; align-items:center; gap:8px;"><input type="radio" name="ttl-opt" value="86400" ${state.ttl === 86400 ? 'checked' : ''}> 24 Hours</label>
            </div>
            <div style="display:flex; justify-content:flex-end; margin-top:8px;">
                <button type="button" class="linen-btn" onclick="applyTtlFromModal()">Apply Timer</button>
            </div>
        </div>
    `);
}

function applyTtlFromModal() {
    const selected = document.querySelector('input[name="ttl-opt"]:checked');
    if (selected) {
        state.ttl = parseInt(selected.value, 10) || 300;
        closeModal();
    }
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
