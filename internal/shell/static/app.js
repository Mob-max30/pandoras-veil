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

  // In-Memory Per-Channel Message Store
  const channelHistories = {
    '#Development': []
  };

  async function fetchIdentity() {
    try {
      const res = await fetch('/api/identity');
      const data = await res.json();
      if (data.initialized && data.handle) {
        currentHostHandle = data.handle;
        currentFingerprint = data.fingerprint || '';
        updateIdentityUI();
        initLiveStream();
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
    const input = prompt('Enter your preferred device handle (e.g. Ujwal, Bob, PV-Alice):', currentHostHandle === 'PV-DEVICE' ? '' : currentHostHandle);
    if (!input || !input.trim()) return;
    const chosen = input.trim();
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
        initLiveStream();
      }
    } catch (e) {
      alert('Failed to register handle: ' + e.message);
    }
  }

  if (changeHandleBtn) {
    changeHandleBtn.addEventListener('click', promptForHandle);
  }

  // 1. Dynamic Channel Management (Add / Select / Remove)
  function addChannelItem(rawName, isGroup, autoSelect = true) {
    let name = rawName.trim();
    if (isGroup) {
      if (!name.startsWith('#')) name = '#' + name;
    }

    const list = isGroup ? groupList : dmList;
    const emptyHint = list.querySelector('.empty-hint');
    if (emptyHint) emptyHint.remove();

    const attr = isGroup ? 'data-group' : 'data-handle';
    let existing = list.querySelector(`[${attr}="${name}"]`);
    if (existing) {
      if (autoSelect) existing.click();
      return;
    }

    const li = document.createElement('li');
    li.className = 'channel-item';
    if (isGroup) {
      li.dataset.group = name;
      li.innerHTML = `
        <span class="channel-label"><span class="hash">#</span>${escapeHTML(name.replace('#', ''))} <span class="dot online"></span></span>
        <button class="remove-channel-btn" title="Remove room">×</button>
      `;
    } else {
      li.dataset.handle = name;
      li.innerHTML = `
        <span class="channel-label"><span class="dot online"></span> ${escapeHTML(name)}</span>
        <button class="remove-channel-btn" title="Close chat">×</button>
      `;
    }

    li.onclick = (e) => {
      if (e.target.classList.contains('remove-channel-btn')) return;
      document.querySelectorAll('.channel-item').forEach(i => i.classList.remove('active'));
      li.classList.add('active');
      activeChannel = name;
      renderCurrentChannel();
      if (!isGroup) {
        checkOfflineInbox(name);
      }
    };

    const removeBtn = li.querySelector('.remove-channel-btn');
    if (removeBtn) {
      removeBtn.onclick = (e) => {
        e.stopPropagation();
        li.remove();
        delete channelHistories[name];

        if (list.querySelectorAll('.channel-item').length === 0) {
          list.innerHTML = `<li class="empty-hint">(No ${isGroup ? 'groups' : 'active chats'} yet)</li>`;
        }

        if (activeChannel === name) {
          const first = document.querySelector('.channel-item');
          if (first) {
            first.click();
          } else {
            addChannelItem('#Development', true, true);
          }
        }
      };
    }

    list.appendChild(li);

    if (!channelHistories[name]) {
      channelHistories[name] = [];
    }

    if (autoSelect) {
      li.click();
    }
  }

  // 2. Live SSE Stream Connection to Relay Backend
  let streamConnected = false;
  function initLiveStream() {
    if (streamConnected) return;
    streamConnected = true;

    const eventSource = new EventSource('/api/stream');

    eventSource.onmessage = e => {
      try {
        const data = JSON.parse(e.data);
        if (!data || !data.sender) return;

        const senderHandle = data.sender.toUpperCase();
        addChannelItem(senderHandle, false, false);

        if (!channelHistories[senderHandle]) {
          channelHistories[senderHandle] = [];
        }

        const timeStr = data.timestamp || new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
        const msgObj = {
          sender: senderHandle,
          text: data.text || '',
          isFile: data.isFile || false,
          filename: data.filename || '',
          fileSize: data.fileSize || 0,
          savePath: data.savePath || '',
          time: timeStr,
          isYou: false
        };

        channelHistories[senderHandle].push(msgObj);

        if (activeChannel.toUpperCase() === senderHandle) {
          renderCurrentChannel();
        }
      } catch (err) {
        console.error('Failed to parse incoming stream message:', err);
      }
    };

    eventSource.onerror = err => {
      console.warn('Stream disconnected, retrying...', err);
    };
  }

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

  async function renderCurrentChannel() {
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
          const downloadNote = msg.savePath ? `<br><small style="color:var(--cyan-primary);">Saved to ${escapeHTML(msg.savePath)}</small>` : '';
          row.innerHTML = `
            <div class="bubble-meta">[${msg.time}] ${msg.isYou ? '<span class="you-badge">[YOU]</span>' : `<span class="sender-name">${escapeHTML(msg.sender)}</span>`}</div>
            <div class="chat-bubble ${msg.isYou ? 'right-bubble' : 'left-bubble'}">
              📁 <strong>[FILE RECEIVED]</strong> ${escapeHTML(msg.filename)} (${Math.round(msg.fileSize / 1024)} KB)${downloadNote}
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
      if (recipFpVal) {
        recipFpVal.textContent = 'Verifying...';
        try {
          const res = await fetch(`/api/lookup?handle=${encodeURIComponent(activeChannel)}`);
          if (res.ok) {
            const data = await res.json();
            recipFpVal.textContent = data.fingerprint || 'Verified';
          } else {
            recipFpVal.textContent = 'Unregistered';
          }
        } catch (e) {
          recipFpVal.textContent = 'Offline';
        }
      }
    } else {
      if (recipFpRow) recipFpRow.style.display = 'none';
    }
    scrollToBottom();
  }

  async function checkOfflineInbox(handle) {
    try {
      const res = await fetch(`/api/inbox?sender=${encodeURIComponent(handle)}`);
      if (res.ok) {
        const msgs = await res.json();
        if (Array.isArray(msgs) && msgs.length > 0) {
          if (!channelHistories[handle]) {
            channelHistories[handle] = [];
          }
          msgs.forEach(m => {
            channelHistories[handle].push({
              sender: m.sender || handle,
              text: m.text || '',
              isFile: m.isFile || false,
              filename: m.filename || '',
              fileSize: m.fileSize || 0,
              savePath: m.savePath || '',
              time: m.timestamp || 'Offline',
              isYou: false
            });
          });
          if (activeChannel.toUpperCase() === handle.toUpperCase()) {
            renderCurrentChannel();
          }
        }
      }
    } catch (e) {
      console.warn('Failed to fetch offline inbox:', e);
    }
  }

  // Initialize Default Group
  addChannelItem('#Development', true, true);
  fetchIdentity();

  // Add DM Button
  addDmBtn.addEventListener('click', () => {
    const handle = prompt('Enter recipient handle to DM (e.g., Ujwal, Bob, Alice):');
    if (!handle || !handle.trim()) return;
    const cleanHandle = handle.trim();
    addChannelItem(cleanHandle, false, true);
  });

  // Add Group Button
  addGroupBtn.addEventListener('click', () => {
    const name = prompt('Enter group chat room name (e.g., #Alpha_Team):');
    if (!name || !name.trim()) return;
    let cleanGroup = name.trim();
    if (!cleanGroup.startsWith('#')) cleanGroup = '#' + cleanGroup;
    addChannelItem(cleanGroup, true, true);
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
    input.onchange = async e => {
      const file = e.target.files[0];
      if (!file) return;

      const reader = new FileReader();
      reader.onload = async () => {
        const result = reader.result;
        const b64Data = result.split(',')[1];

        const payload = {
          target: activeChannel,
          isGroup: activeChannel.startsWith('#'),
          file: {
            filename: file.name,
            dataB64: b64Data
          },
          ttl: activeTTL,
          burn: isBurnActive
        };

        try {
          const res = await fetch('/api/send', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
          });
          let data = {};
          try {
            data = await res.json();
          } catch(e) {
            data = { success: false, error: 'Server response error' };
          }

          if (res.ok && data.success) {
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
          } else {
            alert(data.error || 'Failed to send file');
          }
        } catch (err) {
          alert('Network error uploading file: ' + err.message);
        }
      };
      reader.readAsDataURL(file);
    };
    input.click();
  }

  // Sending Messages
  cmdInput.addEventListener('keydown', async e => {
    if (e.key === 'Enter') {
      const text = cmdInput.value.trim();
      if (!text) return;

      if (text === '/f' || text === '/file' || text === '/attach') {
        triggerFileAttachment();
        cmdInput.value = '';
        return;
      }

      const payload = {
        target: activeChannel,
        isGroup: activeChannel.startsWith('#'),
        text: text,
        ttl: activeTTL,
        burn: isBurnActive
      };

      try {
        const res = await fetch('/api/send', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload)
        });
        let data = {};
        try {
          data = await res.json();
        } catch(e) {
          data = { success: false, error: 'Server response error' };
        }

        if (res.ok && data.success) {
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
        } else {
          alert(data.error || 'Failed to send message');
        }
      } catch (err) {
        alert('Network error sending message: ' + err.message);
      }
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
