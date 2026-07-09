let globalRadioSocket = null; 
let chatSocket = null;        
let currentRoomId = 0; 

// Cache Variables
let isContactsLoaded = false;
let isRoomsLoaded = false;
let isPlaylistLoaded = false;

let isSyncing = false; let myUserCode = ""; let myUserName = ""; let myUserAvatar = "";
let syncPos = 0; let syncTime = 0; let allPlaylistsData = []; let serverPlaylistArray = [];
let currentTrackIndex = -1; let isShuffleEnabled = false; let isSeeking = false; let isSoloMode = false; 

const svgPlaySolid = `<svg viewBox="0 0 24 24" fill="currentColor" width="28" height="28"><path d="M8 5v14l11-7z"/></svg>`;
const svgPauseSolid = `<svg viewBox="0 0 24 24" fill="currentColor" width="28" height="28"><path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z"/></svg>`;
const svgPlayIcon = `<svg viewBox="0 0 24 24" fill="currentColor" class="m-play-icon"><path d="M8 5v14l11-7z"/></svg>`;

function initChat(userCode, fullName, avatarUrl) {
    myUserCode = userCode; myUserName = fullName; myUserAvatar = avatarUrl;
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    
    globalRadioSocket = new WebSocket(`${protocol}//${window.location.host}/chat/ws?room_id=1`);
    globalRadioSocket.onmessage = function(e) {
        const msg = JSON.parse(e.data);
        if (msg.message_type.startsWith("audio_")) handleRadioMessage(msg);
    };

    loadRooms();
    showContacts();
    setupAudioPlayer();
}

function switchLeftTab(tabName) {
    document.querySelectorAll('.ms-tab').forEach(t => t.classList.remove('active'));
    document.querySelectorAll('.ms-list-pane').forEach(c => c.classList.remove('active'));
    document.querySelector(`[onclick="switchLeftTab('${tabName}')"]`).classList.add('active');
    document.getElementById(`left-${tabName}`).classList.add('active');
}

function showView(viewId) {
    document.querySelectorAll('.ms-view').forEach(v => {
        v.classList.remove('active');
        setTimeout(() => { if(!v.classList.contains('active')) v.style.display = 'none'; }, 300);
    });
    const target = document.getElementById(viewId);
    target.style.display = 'flex';
    setTimeout(() => target.classList.add('active'), 10);
}

function openMusicView() {
    showView('view-music');
    document.querySelectorAll('.ms-tab').forEach(t => t.classList.remove('active'));
    document.querySelector(`[onclick="openMusicView()"]`).classList.add('active');
    document.querySelectorAll('.ms-list-item').forEach(el => el.classList.remove('active'));
    if (!isPlaylistLoaded) loadServerPlaylist();
}

function connectChatSocket(roomId, roomName, avatarHtml) {
    if (chatSocket) { chatSocket.onclose = null; chatSocket.close(); }
    currentRoomId = roomId;
    
    showView('view-chat');
    document.getElementById("active-chat-name").innerText = roomName;
    document.getElementById("active-chat-avatar").innerHTML = avatarHtml;
    
    document.querySelectorAll('.ms-list-item').forEach(el => el.classList.remove('active'));
    const activeEl = document.getElementById('room-item-' + roomId);
    if(activeEl) activeEl.classList.add('active');

    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    chatSocket = new WebSocket(`${protocol}//${window.location.host}/chat/ws?room_id=${roomId}`);
    chatSocket.onopen = () => loadChatHistory();
    chatSocket.onmessage = function (event) {
        const msg = JSON.parse(event.data);
        if (!msg.message_type.startsWith("audio_")) handleChatMessage(msg);
    };
    chatSocket.onclose = () => setTimeout(() => connectChatSocket(currentRoomId, roomName, avatarHtml), 3000);
}

// ==========================================
// چت و پیام رسان
// ==========================================
function handleChatMessage(msg) {
    if (msg.message_type === "delete_msg") {
        const el = document.getElementById("msg-" + msg.message_text);
        if(el) el.remove();
    } else { renderMessage(msg); }
}

function renderMessage(msg) {
    const chatBox = document.getElementById("chat-messages");
    if(!chatBox) return;
    const emptyState = chatBox.querySelector('.ms-empty-state');
    if(emptyState) emptyState.remove();

    const rowDiv = document.createElement("div"); rowDiv.classList.add("chat-message-row");
    rowDiv.id = "msg-" + msg.id; 

    const isSelf = msg.sender_id === myUserCode;
    if (isSelf) rowDiv.classList.add("self"); else rowDiv.classList.add("other");

    const avatarEl = document.createElement("div"); avatarEl.classList.add("ms-avatar-placeholder");
    avatarEl.style.width = "38px"; avatarEl.style.height = "38px";
    if (msg.sender_avatar && msg.sender_avatar !== "") avatarEl.innerHTML = `<img src="${msg.sender_avatar}" style="width:100%; height:100%; border-radius:50%; object-fit:cover;">`;
    else avatarEl.innerHTML = `👤`;
    
    const msgContainer = document.createElement("div");
    if (!isSelf && currentRoomId === 1) { 
        const nameDiv = document.createElement("div"); nameDiv.classList.add("sender-name"); nameDiv.innerText = msg.sender_name || msg.sender_id; msgContainer.appendChild(nameDiv);
    }

    const boxDiv = document.createElement("div"); boxDiv.classList.add("chat-message-box");
    if (msg.message_type === "file") {
        boxDiv.innerHTML = `<a href="${msg.media_url}" target="_blank" class="chat-file-link">📎 ${msg.message_text}</a>`;
    } else { boxDiv.innerText = msg.message_text; }
    
    msgContainer.appendChild(boxDiv);
    rowDiv.appendChild(avatarEl); rowDiv.appendChild(msgContainer);
    
    if (isSelf && msg.id > 0) {
        const delBtn = document.createElement("button"); delBtn.className = "del-msg-btn"; delBtn.title = "حذف پیام";
        delBtn.innerHTML = `🗑️`; delBtn.onclick = () => deleteMessage(msg.id);
        rowDiv.appendChild(delBtn);
    }

    chatBox.appendChild(rowDiv); chatBox.scrollTop = chatBox.scrollHeight;
}

function deleteMessage(msgId) {
    if(confirm("آیا از حذف این پیام مطمئن هستید؟")) fetch(`/chat/delete-message?msg_id=${msgId}&user_id=${myUserCode}`);
}

function sendChatMessage() {
    const input = document.getElementById("chat-input"); const text = input.value.trim();
    if (text === "" || !chatSocket) return;
    chatSocket.send(JSON.stringify({ room_id: currentRoomId, sender_id: myUserCode, sender_name: myUserName, sender_avatar: myUserAvatar, message_text: text, message_type: "text", media_url: "" }));
    input.value = "";
}

const chatFileInput = document.getElementById("chat-file-input");
if(chatFileInput) {
    chatFileInput.addEventListener("change", function() {
        if (this.files.length === 0) return;
        const formData = new FormData(); formData.append("chat_media", this.files[0]);
        const xhr = new XMLHttpRequest(); xhr.open("POST", "/chat/upload-file", true);
        xhr.onload = function() {
            if (xhr.status === 200) {
                const data = JSON.parse(xhr.responseText);
                chatSocket.send(JSON.stringify({ room_id: currentRoomId, sender_id: myUserCode, sender_name: myUserName, sender_avatar: myUserAvatar, message_text: data.name, message_type: "file", media_url: data.url }));
            } else { alert("خطا در ارسال فایل"); }
            document.getElementById("chat-file-input").value = "";
        };
        xhr.send(formData);
    });
}

async function loadChatHistory() {
    try {
        const response = await fetch(`/chat/history?room_id=${currentRoomId}`);
        const messages = await response.json();
        const chatBox = document.getElementById("chat-messages");
        if(chatBox) {
            chatBox.innerHTML = ""; 
            if (messages && messages.length > 0) messages.forEach(msg => { handleChatMessage(msg); });
            else chatBox.innerHTML = `<div class="ms-empty-state"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z"/></svg>اولین پیام را ارسال کنید!</div>`;
        }
    } catch (e) { console.error(e); }
}

async function loadRooms() {
    if (isRoomsLoaded) return; 
    try {
        const res = await fetch(`/chat/rooms?user_id=${myUserCode}`);
        const rooms = await res.json();
        const container = document.getElementById("left-chats");
        if(!container) return;
        
        container.innerHTML = "";
        if (rooms && rooms.length > 0) {
            rooms.forEach(room => {
                if (room.id === 1) return; 
                const div = document.createElement("div"); div.className = "ms-list-item";
                div.id = 'room-item-' + room.id;
                
                let avatarHtml = `<div class="ms-avatar-placeholder">👥</div>`;
                if(room.room_type === "direct" && room.avatar !== "") avatarHtml = `<img src="${room.avatar}" class="ms-avatar-placeholder">`;
                else if(room.room_type === "direct") avatarHtml = `<div class="ms-avatar-placeholder">👤</div>`;

                div.innerHTML = `${avatarHtml}<div class="ms-list-info"><div class="ms-list-name">${room.name}</div></div>`;
                div.onclick = () => connectChatSocket(room.id, room.name, avatarHtml);
                container.appendChild(div);
            });
            isRoomsLoaded = true;
        } else {
            container.innerHTML = `<div style="text-align:center; color:#64748b; font-size:12px; margin-top:20px;">هیچ گفتگویی یافت نشد.</div>`;
        }
    } catch (e) { console.error("Error loading rooms", e); }
}

async function showContacts() {
    if (isContactsLoaded) return; 
    try {
        const container = document.getElementById("left-contacts");
        const res = await fetch(`/chat/contacts`);
        const contacts = await res.json();
        container.innerHTML = "";
        
        if(contacts && contacts.length > 0) {
            contacts.forEach(c => {
                if(c.code === myUserCode) return;
                const div = document.createElement("div"); div.className = "ms-list-item";
                let avatarHtml = c.avatar !== "" ? `<img src="${c.avatar}" class="ms-avatar-placeholder">` : `<div class="ms-avatar-placeholder">👤</div>`;
                div.innerHTML = `${avatarHtml}<div class="ms-list-info"><div class="ms-list-name">${c.name}</div></div>`;
                div.onclick = async () => {
                    const req = await fetch("/chat/create-room", { method: "POST", body: JSON.stringify({ creator_id: myUserCode, target_id: c.code, room_type: "direct" }) });
                    const data = await req.json();
                    connectChatSocket(data.room_id, c.name, avatarHtml); 
                    switchLeftTab('chats'); isRoomsLoaded = false; loadRooms(); 
                };
                container.appendChild(div);
            });
            isContactsLoaded = true;
        } else { container.innerHTML = `<div style="text-align:center; color:#64748b; font-size:12px; margin-top:20px;">مخاطبی یافت نشد.</div>`; }
    } catch (e) { 
        document.getElementById("left-contacts").innerHTML = `<div style="text-align:center; color:#ef4444; font-size:12px; margin-top:20px;">خطا در ارتباط با سرور.</div>`;
    }
}

// ==========================================
// منطق موزیک (Desktop App Style)
// ==========================================
function handleRadioMessage(msg) {
    if (isSoloMode) return; 
    if (msg.message_type === "audio_sync") syncAudioPlayer(msg);
    else if (msg.message_type === "audio_new") {
        const audioEl = document.getElementById("shared-audio");
        audioEl.src = msg.media_url; document.getElementById("now-playing-text").innerText = msg.message_text || "آهنگ انتخاب شده";
        document.getElementById("btn-play-pause").innerHTML = svgPauseSolid; 
        if(!isPlaylistLoaded) loadServerPlaylist().then(() => { currentTrackIndex = serverPlaylistArray.findIndex(track => track.url === msg.media_url); });
        
        audioEl.onloadedmetadata = function() {
            isSyncing = true; audioEl.currentTime = 0; let playPromise = audioEl.play();
            if (playPromise !== undefined) {
                playPromise.then(() => { document.getElementById("join-audio-btn").style.display = "none"; setTimeout(() => { isSyncing = false; }, 50); })
                           .catch(e => { syncPos = 0; syncTime = performance.now(); document.getElementById("join-audio-btn").style.display = "inline-block"; isSyncing = false; document.getElementById("btn-play-pause").innerHTML = svgPlaySolid; });
            }
        };
    } else if (msg.message_type === "audio_init") {
        try {
            const state = JSON.parse(msg.message_text); const audioEl = document.getElementById("shared-audio");
            if (state.url) {
                audioEl.src = state.url; document.getElementById("now-playing-text").innerText = state.name || "آهنگ در حال پخش"; 
                audioEl.onloadedmetadata = function() {
                    syncPos = state.pos; syncTime = performance.now();
                    if (state.status === "play") {
                        audioEl.currentTime = syncPos + ((performance.now() - syncTime) / 1000); document.getElementById("btn-play-pause").innerHTML = svgPauseSolid; isSyncing = true;
                        let playPromise = audioEl.play();
                        if (playPromise !== undefined) {
                            playPromise.then(() => { document.getElementById("join-audio-btn").style.display = "none"; setTimeout(() => { isSyncing = false; }, 50); })
                                       .catch(e => { document.getElementById("join-audio-btn").style.display = "inline-block"; isSyncing = false; document.getElementById("btn-play-pause").innerHTML = svgPlaySolid; });
                        }
                    } else { audioEl.currentTime = syncPos; syncTime = 0; document.getElementById("btn-play-pause").innerHTML = svgPlaySolid; }
                };
            }
        } catch(e) { console.error(e); }
    }
}

function sendRadioCommand(msgType, text, url) {
    if(!globalRadioSocket) return;
    globalRadioSocket.send(JSON.stringify({ room_id: 1, sender_id: myUserCode, sender_name: myUserName, sender_avatar: myUserAvatar, message_type: msgType, message_text: text, media_url: url }));
}

function togglePlayPause() { const audioEl = document.getElementById("shared-audio"); if (!audioEl.src) return; if (audioEl.paused) audioEl.play(); else audioEl.pause(); }
function syncAudioPlayer(msg) {
    if (msg.sender_id === myUserCode) return; 
    const audioEl = document.getElementById("shared-audio"); isSyncing = true;
    if (msg.message_text === "play") {
        syncPos = parseFloat(msg.media_url); syncTime = performance.now(); audioEl.currentTime = syncPos; let playPromise = audioEl.play();
        if (playPromise !== undefined) {
            playPromise.then(() => { document.getElementById("join-audio-btn").style.display = "none"; document.getElementById("btn-play-pause").innerHTML = svgPauseSolid; })
                       .catch(e => { document.getElementById("join-audio-btn").style.display = "inline-block"; document.getElementById("btn-play-pause").innerHTML = svgPlaySolid; });
        }
    } else if (msg.message_text === "pause") { audioEl.pause(); syncPos = parseFloat(msg.media_url); syncTime = 0; audioEl.currentTime = syncPos; document.getElementById("btn-play-pause").innerHTML = svgPlaySolid; }
    setTimeout(() => { isSyncing = false; }, 50); 
}
function joinAudioSync() {
    const audioEl = document.getElementById("shared-audio");
    if (syncTime > 0) audioEl.currentTime = syncPos + ((performance.now() - syncTime) / 1000); else audioEl.currentTime = syncPos;
    isSyncing = true;
    audioEl.play().then(() => { document.getElementById("join-audio-btn").style.display = "none"; document.getElementById("btn-play-pause").innerHTML = svgPauseSolid; setTimeout(() => { isSyncing = false; }, 50); }).catch(e => console.error(e));
}

function setupAudioPlayer() {
    const audioEl = document.getElementById("shared-audio"); const progressEl = document.getElementById("audio-progress"); 
    const timeCur = document.getElementById("player-time-current"); const timeTot = document.getElementById("player-time-total"); 
    const fillEl = document.getElementById("progress-filled");
    
    audioEl.addEventListener("timeupdate", () => { 
        if (!isSeeking && audioEl.duration) { 
            const pct = (audioEl.currentTime / audioEl.duration) * 100; 
            progressEl.value = pct; fillEl.style.width = pct + "%";
            timeCur.innerText = formatTime(audioEl.currentTime); timeTot.innerText = formatTime(audioEl.duration); 
        } 
    });
    
    audioEl.addEventListener("ended", () => { if (!isSyncing) playNextTrack(); });
    progressEl.addEventListener("mousedown", () => { isSeeking = true; });
    progressEl.addEventListener("input", () => { 
        if (audioEl.duration) {
            const pct = progressEl.value; fillEl.style.width = pct + "%";
            timeCur.innerText = formatTime((pct / 100) * audioEl.duration); 
        } 
    });
    progressEl.addEventListener("mouseup", () => { isSeeking = false; if (audioEl.duration) { const seekTime = (progressEl.value / 100) * audioEl.duration; audioEl.currentTime = seekTime; if (!isSoloMode) sendRadioCommand("audio_sync", "play", seekTime.toString()); } });
    
    const fileInput = document.getElementById("audio-upload");
    if(fileInput) {
        fileInput.addEventListener("change", function() {
            if (this.files.length === 0) return;
            const formData = new FormData(); const file = this.files[0]; formData.append("audio_file", file); formData.append("room_id", 1);
            
            const xhr = new XMLHttpRequest(); xhr.open("POST", "/chat/upload-audio", true);
            xhr.onload = function() {
                if (xhr.status === 200) {
                    try { const data = JSON.parse(xhr.responseText); if (data.url) { isPlaylistLoaded = false; loadServerPlaylist().then(() => { playFromServer(data.url, file.name); }); } } catch (e) {}
                } else { alert("خطا در آپلود"); }
                fileInput.value = ""; 
            };
            xhr.send(formData);
        });
    }

    audioEl.addEventListener("play", () => { document.getElementById("btn-play-pause").innerHTML = svgPauseSolid; if (isSyncing || isSoloMode) return; sendRadioCommand("audio_sync", "play", audioEl.currentTime.toString()); });
    audioEl.addEventListener("pause", () => { document.getElementById("btn-play-pause").innerHTML = svgPlaySolid; if (isSyncing || isSoloMode) return; sendRadioCommand("audio_sync", "pause", audioEl.currentTime.toString()); });
}

function playFromServer(url, name) {
    if (isSoloMode) {
        const audioEl = document.getElementById("shared-audio"); audioEl.src = url; document.getElementById("now-playing-text").innerText = name; updateTrackIndexByUrl(url); audioEl.play();
    } else { sendRadioCommand("audio_new", name, url); document.getElementById("now-playing-text").innerText = name; }
}

function formatTime(seconds) { if (isNaN(seconds)) return "00:00"; const m = Math.floor(seconds / 60); const s = Math.floor(seconds % 60); return (m < 10 ? "0" + m : m) + ":" + (s < 10 ? "0" + s : s); }
function toggleSoloMode() { isSoloMode = !isSoloMode; const btn = document.getElementById("btn-solo"); if (isSoloMode) { btn.classList.add("active"); document.getElementById("join-audio-btn").style.display = "none"; } else { btn.classList.remove("active"); document.getElementById("join-audio-btn").style.display = "inline-block"; } }
function toggleShuffle() { isShuffleEnabled = !isShuffleEnabled; const btn = document.getElementById("btn-shuffle"); if (isShuffleEnabled) { btn.classList.add("active"); } else { btn.classList.remove("active"); } }
function playNextTrack() { if (serverPlaylistArray.length === 0) return; let nextIndex = isShuffleEnabled ? Math.floor(Math.random() * serverPlaylistArray.length) : (currentTrackIndex + 1); if (nextIndex >= serverPlaylistArray.length) nextIndex = 0; playFromServer(serverPlaylistArray[nextIndex].url, serverPlaylistArray[nextIndex].name); }
function playPrevTrack() { if (serverPlaylistArray.length === 0) return; let prevIndex = currentTrackIndex - 1; if (prevIndex < 0) prevIndex = serverPlaylistArray.length - 1; playFromServer(serverPlaylistArray[prevIndex].url, serverPlaylistArray[prevIndex].name); }

// ==========================================
// منطق موزیک (Search & Albums Datalist)
// ==========================================

async function loadServerPlaylist() {
    try {
        const response = await fetch(`/chat/playlist`); const tracks = await response.json(); allPlaylistsData = tracks || [];
        const playlistNames = [...new Set(allPlaylistsData.map(t => t.playlist))]; 
        
        // 🚀 پر کردن لیست کشویی (Datalist) برای آپلود و ویرایش آسان
        const datalist = document.getElementById("album-list-options");
        if (datalist) {
            datalist.innerHTML = "";
            playlistNames.forEach(name => {
                if (name !== "پیش‌فرض") datalist.innerHTML += `<option value="${name}">`;
            });
        }

        // رندر کردن گالری آلبوم‌ها
        const selectorContainer = document.getElementById("playlist-cards-container");
        if (selectorContainer) {
            selectorContainer.innerHTML = `
                <div class="album-card grad-0 active" id="album-card-all" onclick="filterPlaylist('all')">
                    <div class="album-icon">🎵</div>
                    <span>همه آهنگ‌ها</span>
                </div>
            `;
            playlistNames.forEach((name, index) => {
                let gradNum = (index % 5) + 1;
                let icon = name.includes("پادکست") ? "🎙️" : "💿";
                selectorContainer.innerHTML += `
                    <div class="album-card grad-${gradNum}" id="album-card-${name}" onclick="filterPlaylist('${name}')">
                        <div class="album-icon">${icon}</div>
                        <span>${name}</span>
                    </div>
                `;
            });
        }
        filterPlaylist("all");
        isPlaylistLoaded = true;
    } catch (e) { console.error(e); }
}

function filterPlaylist(playlistName) {
    document.querySelectorAll('.album-card').forEach(c => c.classList.remove('active'));
    const activeCard = document.getElementById(playlistName === 'all' ? 'album-card-all' : 'album-card-' + playlistName);
    if(activeCard) activeCard.classList.add('active');

    document.getElementById("current-album-title").innerText = playlistName === 'all' ? "همه آهنگ‌ها" : "آلبوم: " + playlistName;

    serverPlaylistArray = playlistName === "all" ? allPlaylistsData : allPlaylistsData.filter(t => t.playlist === playlistName);
    renderTrackList(serverPlaylistArray);
}

// 🚀 جستجوی کاملاً زنده (Live Search) 
function searchMusic(query) {
    if(!query || query.trim() === "") {
        // بازگشت به حالت آلبوم انتخاب شده قبلی
        const activeCard = document.querySelector('.album-card.active');
        const activeAlbum = activeCard ? activeCard.id.replace('album-card-', '') : 'all';
        filterPlaylist(activeAlbum);
        return;
    }
    
    query = query.toLowerCase().trim();
    // جستجو در نام آهنگ یا نام آلبوم
    const filteredTracks = allPlaylistsData.filter(t => t.name.toLowerCase().includes(query) || t.playlist.toLowerCase().includes(query));
    
    // غیرفعال کردن استایل کارت‌های آلبوم در زمان سرچ
    document.querySelectorAll('.album-card').forEach(c => c.classList.remove('active'));
    
    document.getElementById("current-album-title").innerText = `نتایج جستجو برای: "${query}"`;
    renderTrackList(filteredTracks);
}

// تابع رندر کردن جدول آهنگ ها
function renderTrackList(tracksArray) {
    const listContainer = document.getElementById("server-playlist"); listContainer.innerHTML = "";
    if (tracksArray.length > 0) {
        tracksArray.forEach((item, index) => {
            const div = document.createElement("div"); div.className = "m-track-item";
            div.innerHTML = `
                <div class="m-col-num"><span class="m-num tabular">${index+1}</span>${svgPlayIcon}</div>
                <div class="m-col-name">
                    <div class="m-track-art"><svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor"><path d="M9 18V5l12-2v13"></path><circle cx="6" cy="18" r="3"></circle><circle cx="18" cy="16" r="3"></circle></svg></div>
                    <span class="m-track-title">${item.name}</span>
                </div>
                <div class="m-col-album">${item.playlist}</div>
                <div class="m-col-action" style="display:flex; justify-content:flex-end;">
                    <button class="btn-edit-track" onclick="event.stopPropagation(); openEditModal('${item.name}', '${item.playlist}')">⚙️ جابجایی</button>
                    <button style="background:rgba(255,255,255,0.1); border:none; color:#fff; padding:6px 12px; border-radius:6px; cursor:pointer; font-weight:bold; font-size:11px;">پخش 🎵</button>
                </div>
            `;
            div.onclick = () => { 
                currentTrackIndex = allPlaylistsData.findIndex(track => track.url === item.url); 
                playFromServer(item.url, item.name); 
            }; 
            listContainer.appendChild(div);
        });
    } else { listContainer.innerHTML = "<div style='text-align:center; color:#64748b; font-size:13px; margin-top:30px; font-weight:bold;'>آهنگی یافت نشد 🧐</div>"; }
}

// ==========================================
// منطق مودال جابجایی آهنگ ها
// ==========================================
function openEditModal(fileName, oldPlaylist) {
    document.getElementById('edit-modal-track-name').innerText = "آهنگ: " + fileName;
    document.getElementById('edit-modal-file-name').value = fileName;
    document.getElementById('edit-modal-old-album').value = oldPlaylist;
    
    // اگر آلبوم قبلی پیش‌فرض بوده، کادر خالی باشد، در غیر این صورت نامش را بنویسیم
    document.getElementById('edit-modal-new-album').value = (oldPlaylist === "پیش‌فرض") ? "" : oldPlaylist;
    
    document.getElementById('edit-album-modal').style.display = 'flex';
}

function closeEditModal() {
    document.getElementById('edit-album-modal').style.display = 'none';
}

function submitMoveAudio() {
    const fileName = document.getElementById('edit-modal-file-name').value;
    const oldPlaylist = document.getElementById('edit-modal-old-album').value;
    let newPlaylist = document.getElementById('edit-modal-new-album').value.trim();
    
    if(!newPlaylist) { alert("لطفاً نام آلبوم جدید را وارد کنید."); return; }
    if(newPlaylist === oldPlaylist) { closeEditModal(); return; } // نیازی به تغییر نیست
    
    const formData = new FormData();
    formData.append("file_name", fileName);
    formData.append("old_playlist", oldPlaylist);
    formData.append("new_playlist", newPlaylist);
    
    const xhr = new XMLHttpRequest();
    xhr.open("POST", "/chat/move-audio", true);
    xhr.onload = function() {
        if(xhr.status === 200) {
            closeEditModal();
            isPlaylistLoaded = false; // این کار باعث رفرش کش آلبوم ها می شود
            loadServerPlaylist().then(() => {
                filterPlaylist(newPlaylist); // مستقیماً وارد آلبومی که تازه ساخته شده می‌شویم!
            });
        } else { alert("خطا در جابجایی فایل سرور"); }
    };
    xhr.send(formData);
}

// 🚀 آپدیت رویداد آپلود برای ارسال نام پوشه
const audioFileInput = document.getElementById("audio-upload");
if(audioFileInput) {
    audioFileInput.addEventListener("change", function() {
        if (this.files.length === 0) return;
        const albumInput = document.getElementById('upload-album-name').value; // دریافت نام آلبوم تایپ شده
        
        const formData = new FormData(); 
        const file = this.files[0]; 
        formData.append("audio_file", file); 
        formData.append("room_id", 1);
        formData.append("playlist_name", albumInput); // ارسال به سرور Go
        
        const xhr = new XMLHttpRequest(); xhr.open("POST", "/chat/upload-audio", true);
        xhr.onload = function() {
            if (xhr.status === 200) {
                try { 
                    const data = JSON.parse(xhr.responseText); 
                    if (data.url) { 
                        isPlaylistLoaded = false; 
                        loadServerPlaylist().then(() => { playFromServer(data.url, file.name); }); 
                    } 
                } catch (e) {}
            } else { alert("خطا در آپلود. بررسی کنید حجم فایل بیشتر از ۵۰ مگابایت نباشد."); }
            audioFileInput.value = ""; 
        };
        xhr.send(formData);
    });
}