document.addEventListener('DOMContentLoaded', () => {
  const cmdInput = document.getElementById('cmd-input');
  const fileBtn = document.getElementById('file-picker-btn');
  const msgContainer = document.getElementById('message-container');
  const headerChannelTitle = document.getElementById('header-channel-title');
  const cmdChannelTag = document.getElementById('cmd-channel-tag');
  const recipMeta = document.getElementById('recip-meta');
  const burnToggle = document.getElementById('burn-toggle');
  const burnText = document.getElementById('burn-text');
  const ttlBtns = document.querySelectorAll('.ttl-btn');
  const dmList = document.getElementById('dm-list');
  const groupList = document.getElementById('group-list');
  const addDmBtn = document.getElementById('add-dm-btn');
  const addGroupBtn = document.getElementById('add-group-btn');

  let activeChannel = '#Development';
  let activeTTL = 300;
  let isBurnActive = true;
  let currentHostHandle = 'PV-DEVICE';
  let currentFingerprint = '';

  const hostHandleEl = document.getElementById('host-handle');
  const hostFingerprintEl = document.getElementById('host-fingerprint');
  const metaHostEl = document.getElementById('meta-host');
  const changeHandleBtn = document.getElementById('change-handle-btn');

  async function fetchIdentity() {
    try {
      const res = await fetch('/api/identity');
      const data = await res.json();
      if (data.initialized && data.handle) {
        currentHostHandle = data.handle;
        currentFingerprint = data.fingerprint || '';
        updateIdentityUI();
      } else {
        promptForHandle();
      }
    } catch (e) {
      console.warn('Identity API offline:', e);
    }
  }

  function updateIdentityUI() {
    if (hostHandleEl) hostHandleEl.textContent = currentHostHandle;
    if (hostFingerprintEl) hostFingerprintEl.textContent = currentFingerprint || '🔒 Active';
    if (metaHostEl) metaHostEl.textContent = currentHostHandle;
  }

  async function promptForHandle() {
    const input = prompt('Enter your preferred device handle (e.g., PV-UJWAL):', currentHostHandle === 'PV-DEVICE' ? 'PV-USER' : currentHostHandle);
    if (!input || !input.trim()) return;
    let chosen = input.trim();
    if (!chosen.toUpperCase().startsWith('PV-')) {
      chosen = 'PV-' + chosen.toUpperCase();
    }
    try {
      const res = await fetch('/api/init', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ handle: chosen })
      });
      const data = await res.json();
      if (data.success && data.handle) {
        currentHostHandle = data.handle;
        currentFingerprint = data.fingerprint || '';
        updateIdentityUI();
      }
    } catch (e) {
      alert('Failed to register handle: ' + e.message);
    }
  }

  if (changeHandleBtn) {
    changeHandleBtn.addEventListener('click', promptForHandle);
  }

  fetchIdentity();

  // In-Memory Per-Channel Message Store
  const channelHistories = {
    '#Development': [
      { sender: 'PV-UJWAL', text: 'Deployment complete for v1.2.5.', time: '14:32', isYou: false },
      { sender: 'YOU', text: 'Confirmed. Monitoring throughput.', time: '14:38', isYou: true },
      { sender: 'PV-UJWAL', text: 'Need final verification on the protocol patch.', time: '14:40', isYou: false }
    ]
  };

  // TTL Selection
  ttlBtns.forEach(btn => {
    btn.addEventListener('click', () => {
      ttlBtns.forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      activeTTL = parseInt(btn.dataset.ttl, 10);
    });
  });

  // Burn Toggle
  burnToggle.addEventListener('change', () => {
    isBurnActive = burnToggle.checked;
    const burnPill = document.getElementById('burn-pill');
    if (burnPill) {
      if (isBurnActive) {
        burnPill.textContent = '⚡ ARMED (Self-Destruct)';
        burnPill.className = 'burn-status-pill armed';
      } else {
        burnPill.textContent = '⚪ INACTIVE';
        burnPill.className = 'burn-status-pill inactive';
      }
    }
  });

  function renderCurrentChannel() {
    msgContainer.innerHTML = '';
    const history = channelHistories[activeChannel] || [];

    if (history.length === 0) {
      const isGroup = activeChannel.startsWith('#');
      const systemNotice = document.createElement('div');
      systemNotice.className = 'chat-bubble-row left';
      systemNotice.innerHTML = `
        <div class="bubble-meta">[SYSTEM]</div>
        <div class="chat-bubble left-bubble">
          🔒 <strong>${isGroup ? 'Encrypted Group Room' : 'Encrypted Direct Session'}: ${escapeHTML(activeChannel)}</strong><br>
          Zero-Knowledge Relay active. Type a message or click <strong>/f attach</strong> below.
        </div>
      `;
      msgContainer.appendChild(systemNotice);
    } else {
      history.forEach(msg => {
        const row = document.createElement('div');
        row.className = `chat-bubble-row ${msg.isYou ? 'right' : 'left'}`;
        if (msg.isFile) {
          row.innerHTML = `
            <div class="bubble-meta">[${msg.time}] ${msg.isYou ? '<span class="you-badge">[YOU]</span>' : `<span class="sender-name">${escapeHTML(msg.sender)}</span>`}</div>
            <div class="chat-bubble ${msg.isYou ? 'right-bubble' : 'left-bubble'}">
              📁 [FILE] ${escapeHTML(msg.filename)} (${Math.round(msg.fileSize / 1024)} KB)
            </div>
          `;
        } else {
          row.innerHTML = `
            <div class="bubble-meta">[${msg.time}] ${msg.isYou ? '<span class="you-badge">[YOU]</span>' : `<span class="sender-name">${escapeHTML(msg.sender)}</span>`}</div>
            <div class="chat-bubble ${msg.isYou ? 'right-bubble' : 'left-bubble'}">
              ${escapeHTML(msg.text)}
            </div>
          `;
        }
        msgContainer.appendChild(row);
      });
    }

    headerChannelTitle.textContent = activeChannel;
    cmdChannelTag.textContent = activeChannel;
    recipMeta.textContent = activeChannel;

    const recipFpRow = document.getElementById('recip-fp-row');
    const recipFpVal = document.getElementById('recip-fp-val');
    if (!activeChannel.startsWith('#')) {
      if (recipFpRow) recipFpRow.style.display = 'flex';
      if (recipFpVal) recipFpVal.textContent = '1E42-2834';
    } else {
      if (recipFpRow) recipFpRow.style.display = 'none';
    }
    scrollToBottom();
  }

  function bindChannelEvents() {
    document.querySelectorAll('.channel-item').forEach(item => {
      item.onclick = () => {
        document.querySelectorAll('.channel-item').forEach(i => i.classList.remove('active'));
        item.classList.add('active');
        activeChannel = item.dataset.handle || item.dataset.group;
        renderCurrentChannel();
      };
    });
  }

  bindChannelEvents();
  renderCurrentChannel();

  // Add DM
  addDmBtn.addEventListener('click', () => {
    const handle = prompt('Enter recipient handle (e.g., PV-UJWAL):');
    if (!handle) return;
    const cleanHandle = handle.trim().toUpperCase();
    if (!cleanHandle) return;

    const emptyHint = dmList.querySelector('.empty-hint');
    if (emptyHint) emptyHint.remove();

    let existing = dmList.querySelector(`[data-handle="${cleanHandle}"]`);
    if (!existing) {
      existing = document.createElement('li');
      existing.className = 'channel-item';
      existing.dataset.handle = cleanHandle;
      existing.innerHTML = `<span class="dot online"></span> ${cleanHandle}`;
      dmList.appendChild(existing);
      bindChannelEvents();
    }
    if (!channelHistories[cleanHandle]) {
      channelHistories[cleanHandle] = [];
    }
    existing.click();
  });

  // Add Group
  addGroupBtn.addEventListener('click', () => {
    const name = prompt('Enter group chat name (e.g., #Time_Pass):');
    if (!name) return;
    let cleanGroup = name.trim();
    if (!cleanGroup.startsWith('#')) cleanGroup = '#' + cleanGroup;

    let existing = groupList.querySelector(`[data-group="${cleanGroup}"]`);
    if (!existing) {
      existing = document.createElement('li');
      existing.className = 'channel-item';
      existing.dataset.group = cleanGroup;
      existing.innerHTML = `<span class="hash">#</span>${cleanGroup.replace('#', '')} <span class="dot online"></span>`;
      groupList.appendChild(existing);
      bindChannelEvents();
    }
    if (!channelHistories[cleanGroup]) {
      channelHistories[cleanGroup] = [];
    }
    existing.click();
  });

  function scrollToBottom() {
    setTimeout(() => {
      msgContainer.scrollTop = msgContainer.scrollHeight + 10000;
    }, 50);
  }

  // File Picker Trigger (/f)
  fileBtn.addEventListener('click', () => {
    triggerFileAttachment();
  });

  function triggerFileAttachment() {
    const input = document.createElement('input');
    input.type = 'file';
    input.onchange = e => {
      const file = e.target.files[0];
      if (!file) return;

      const timeStr = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
      const msgObj = {
        sender: 'YOU',
        isFile: true,
        filename: file.name,
        fileSize: file.size,
        time: timeStr,
        isYou: true
      };

      if (!channelHistories[activeChannel]) {
        channelHistories[activeChannel] = [];
      }
      channelHistories[activeChannel].push(msgObj);
      renderCurrentChannel();
    };
    input.click();
  }

  // Sending Messages
  cmdInput.addEventListener('keydown', e => {
    if (e.key === 'Enter') {
      const text = cmdInput.value.trim();
      if (!text) return;

      if (text === '/f' || text === '/file' || text === '/attach') {
        triggerFileAttachment();
        cmdInput.value = '';
        return;
      }

      const timeStr = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
      const msgObj = {
        sender: 'YOU',
        text: text,
        time: timeStr,
        isYou: true
      };

      if (!channelHistories[activeChannel]) {
        channelHistories[activeChannel] = [];
      }
      channelHistories[activeChannel].push(msgObj);
      renderCurrentChannel();

      cmdInput.value = '';
    }
  });

  // Global Keyboard Shortcuts (Ctrl+Q, Ctrl+N, Tab)
  document.addEventListener('keydown', e => {
    // Ctrl+Q or Cmd+Q: Exit App Window
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'q') {
      e.preventDefault();
      window.close();
      return;
    }

    // Ctrl+N: Create New Group
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'n') {
      e.preventDefault();
      addGroupBtn.click();
      return;
    }

    // Tab: Cycle / Switch Active Channel
    if (e.key === 'Tab') {
      e.preventDefault();
      const items = Array.from(document.querySelectorAll('.channel-item'));
      if (items.length <= 1) return;
      const currIdx = items.findIndex(i => i.classList.contains('active'));
      const nextIdx = (currIdx + 1) % items.length;
      items[nextIdx].click();
      return;
    }
  });

  function escapeHTML(str) {
    return str.replace(/[&<>'"]/g, 
      tag => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[tag] || tag)
    );
  }
});
