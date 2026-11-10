// Lobby 3D do PZLink: personagens e objetos vêm da instalação do Project
// Zomboid do próprio jogador (servidos pelo PZLink local), nunca da internet.
import * as THREE from 'three';
import { GLTFLoader } from 'three/addons/loaders/GLTFLoader.js';
import { FBXLoader } from 'three/addons/loaders/FBXLoader.js';

const root = document.getElementById('lobby');
const canvas = document.getElementById('lobby-canvas');
const labels = document.getElementById('lobby-labels');
const msg = document.getElementById('lobby-msg');

const renderer = new THREE.WebGLRenderer({ canvas, antialias: true });
renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
renderer.outputColorSpace = THREE.SRGBColorSpace;
renderer.toneMapping = THREE.ACESFilmicToneMapping;
renderer.toneMappingExposure = 1.05;
renderer.shadowMap.enabled = true;
renderer.shadowMap.type = THREE.PCFShadowMap;

const scene = new THREE.Scene();
const camera = new THREE.PerspectiveCamera(30, 2, 0.05, 80);
// enquadramento: cabeça + placa (~1,3) perto do topo, pés perto da base
const camBase = new THREE.Vector3(0, 0.8, 2.85);
const camTarget = new THREE.Vector3(0, 0.62, -0.35);

// ---------- céu, neblina e luz ----------
function gradientTexture(stops, w = 4, h = 256) {
  const c = document.createElement('canvas'); c.width = w; c.height = h;
  const g = c.getContext('2d'), grd = g.createLinearGradient(0, 0, 0, h);
  stops.forEach(([p, col]) => grd.addColorStop(p, col));
  g.fillStyle = grd; g.fillRect(0, 0, w, h);
  const t = new THREE.CanvasTexture(c); t.colorSpace = THREE.SRGBColorSpace; return t;
}
scene.background = gradientTexture([[0, '#1b1a2e'], [0.45, '#4a3550'], [0.72, '#a2593b'], [0.86, '#d98a4a'], [1, '#2a2420']]);
scene.fog = new THREE.Fog(0x3a2c33, 5, 16);

scene.add(new THREE.HemisphereLight(0x8d7fa6, 0x2b2117, 0.75));
const moon = new THREE.DirectionalLight(0xb7c4ff, 0.9);
moon.position.set(-3, 5, 2.5);
moon.castShadow = true;
moon.shadow.mapSize.set(1024, 1024);
Object.assign(moon.shadow.camera, { left: -4, right: 4, top: 4, bottom: -2, near: 0.5, far: 15 });
scene.add(moon);
// luz de palco vinda da câmera, para os rostos não ficarem no escuro
const key = new THREE.SpotLight(0xffe2c4, 9, 9, 0.55, 0.6, 1.4);
key.position.set(0.4, 2.4, 3.2); key.target.position.set(0, 0.5, -0.3);
scene.add(key, key.target);
const rim = new THREE.DirectionalLight(0xff9a5c, 0.8);
rim.position.set(2, 2, -4);
scene.add(rim);

// ---------- chão ----------
function groundTexture() {
  const c = document.createElement('canvas'); c.width = c.height = 512;
  const g = c.getContext('2d');
  g.fillStyle = '#3a3a24'; g.fillRect(0, 0, 512, 512);
  for (let i = 0; i < 9000; i++) {
    const v = Math.random();
    g.fillStyle = v < 0.5 ? `rgba(70,78,40,${0.3 + Math.random() * 0.4})` : v < 0.8 ? `rgba(48,40,26,${0.4})` : `rgba(95,92,60,${0.35})`;
    const s = 1 + Math.random() * 3;
    g.fillRect(Math.random() * 512, Math.random() * 512, s, s * (1 + Math.random() * 2));
  }
  const t = new THREE.CanvasTexture(c);
  t.wrapS = t.wrapT = THREE.RepeatWrapping; t.repeat.set(14, 14); t.colorSpace = THREE.SRGBColorSpace;
  t.anisotropy = 4;
  return t;
}
const ground = new THREE.Mesh(new THREE.PlaneGeometry(60, 60), new THREE.MeshStandardMaterial({ map: groundTexture(), roughness: 1 }));
ground.rotation.x = -Math.PI / 2; ground.receiveShadow = true; scene.add(ground);
const clearing = new THREE.Mesh(new THREE.CircleGeometry(2.3, 48), new THREE.MeshStandardMaterial({ color: 0x2c2419, roughness: 1, transparent: true, opacity: 0.55 }));
clearing.rotation.x = -Math.PI / 2; clearing.position.set(0, 0.002, -0.6); clearing.receiveShadow = true; scene.add(clearing);

// ---------- pinheiros ao fundo ----------
const treeMat = new THREE.MeshStandardMaterial({ color: 0x1d2a1c, roughness: 1 });
const trunkMat = new THREE.MeshStandardMaterial({ color: 0x2a1d14, roughness: 1 });
function pine(x, z, h) {
  const t = new THREE.Group();
  const trunk = new THREE.Mesh(new THREE.CylinderGeometry(0.04 * h, 0.06 * h, 0.35 * h, 6), trunkMat);
  trunk.position.y = 0.17 * h; t.add(trunk);
  for (let i = 0; i < 3; i++) {
    const c = new THREE.Mesh(new THREE.ConeGeometry((0.42 - i * 0.1) * h, 0.55 * h, 7), treeMat);
    c.position.y = (0.45 + i * 0.28) * h; c.castShadow = true; t.add(c);
  }
  t.position.set(x, 0, z); t.rotation.y = Math.random() * 6; scene.add(t);
}
for (let i = 0; i < 46; i++) {
  const a = Math.PI * (0.05 + 0.9 * Math.random()), r = 5 + Math.random() * 8;
  pine(Math.cos(a) * r * 1.4, -Math.sin(a) * r - 1.5, 1.4 + Math.random() * 1.6);
}

// ---------- fogueira (luz e fagulhas) ----------
const firePos = new THREE.Vector3(0, 0.08, -1.55);
const fireLight = new THREE.PointLight(0xff7a2e, 2.2, 5, 1.6);
fireLight.position.copy(firePos).add(new THREE.Vector3(0, 0.2, 0));
fireLight.castShadow = true; fireLight.shadow.mapSize.set(512, 512);
scene.add(fireLight);
const flame = new THREE.Mesh(new THREE.ConeGeometry(0.07, 0.22, 8, 1, true),
  new THREE.MeshBasicMaterial({ color: 0xffa040, transparent: true, opacity: 0.85, blending: THREE.AdditiveBlending, depthWrite: false }));
flame.position.copy(firePos).add(new THREE.Vector3(0, 0.1, 0)); scene.add(flame);
const SPARKS = 70;
const sparkGeo = new THREE.BufferGeometry();
const sparkPos = new Float32Array(SPARKS * 3), sparkLife = new Float32Array(SPARKS);
for (let i = 0; i < SPARKS; i++) sparkLife[i] = Math.random();
sparkGeo.setAttribute('position', new THREE.BufferAttribute(sparkPos, 3));
const sparks = new THREE.Points(sparkGeo, new THREE.PointsMaterial({ color: 0xffb060, size: 0.018, transparent: true, opacity: 0.9, blending: THREE.AdditiveBlending, depthWrite: false }));
scene.add(sparks);

// ---------- objetos do jogo ----------
const PROPS = [
  // id, posição, rotação Y, altura alvo (o personagem mede ~0,98)
  ['fogueira', [0, 0, -1.55], 0, 0.09],
  ['lenha', [0.42, 0, -1.4], 0.6, 0.1],
  ['barraca', [-2.1, 0, -2.5], 0.5, 0.78],
  ['gerador', [2.05, 0, -2.1], -0.6, 0.34],
  ['caixote', [1.35, 0, -1.25], -0.3, 0.26],
  ['radio', [1.35, 0.26, -1.25], 0.4, 0.1],
  ['caixote', [-1.4, 0, -1.15], 0.25, 0.26],
  ['lampiao', [-1.4, 0.26, -1.15], 0, 0.22],
];
const texLoader = new THREE.TextureLoader();
const propCache = new Map();

function loadProp(id) {
  if (!propCache.has(id)) {
    propCache.set(id, new Promise((resolve, reject) => {
      const loader = new FBXLoader(new THREE.LoadingManager());
      // o FBX pode citar texturas pelo nome; todas vão para a textura do próprio objeto
      loader.manager.setURLModifier(u => u.endsWith('/model') ? u : `/pz/prop/${id}/texture`);
      loader.load(`/pz/prop/${id}/model`, obj => {
        texLoader.load(`/pz/prop/${id}/texture`, tex => { tex.colorSpace = THREE.SRGBColorSpace; resolve({ obj, tex }); },
          undefined, () => resolve({ obj, tex: null }));
      }, undefined, reject);
    }));
  }
  return propCache.get(id);
}

function fitHeight(obj, h) {
  const box = new THREE.Box3().setFromObject(obj);
  const size = box.getSize(new THREE.Vector3());
  const s = h / Math.max(size.y, 1e-4);
  obj.scale.multiplyScalar(s);
  const b2 = new THREE.Box3().setFromObject(obj);
  obj.position.y -= b2.min.y;
  const c = b2.getCenter(new THREE.Vector3());
  obj.position.x -= c.x; obj.position.z -= c.z;
}

async function placeProps() {
  for (const [id, pos, rot, h] of PROPS) {
    try {
      const { obj, tex } = await loadProp(id);
      const o = obj.clone(true);
      o.traverse(m => {
        if (!m.isMesh) return;
        m.castShadow = m.receiveShadow = true;
        m.material = new THREE.MeshStandardMaterial({ map: tex, color: tex ? 0xffffff : 0x8a7a66, roughness: 0.85, metalness: 0.05 });
      });
      const holder = new THREE.Group(); holder.add(o); fitHeight(o, h);
      holder.position.set(...pos); holder.rotation.y = rot; scene.add(holder);
      if (id === 'lampiao') {
        const l = new THREE.PointLight(0xffd08a, 0.9, 2.5, 1.8);
        l.position.set(pos[0], pos[1] + h * 0.7, pos[2]); scene.add(l);
      }
    } catch (e) { /* objeto ausente nesta versão do jogo: segue sem ele */ }
  }
}

// ---------- personagens ----------
const gltfLoader = new GLTFLoader();
const players = new Map(); // seed -> { group, mixer, data, label, ring }
let idleSpeed = 0.38; // o jogo toca o idle a ~38% da velocidade original (AnimSets/player/idle)
const SLOTS = [[0, 0], [-0.7, -0.2], [0.7, -0.2], [-1.36, -0.48], [1.36, -0.48], [-1.95, -0.85], [1.95, -0.85], [0, -0.95]];
const LEVEL_COLOR = { good: 0x5fae5a, ok: 0xd6a531, bad: 0xd4513f, wait: 0x7c8796 };

function ringFor(level) {
  const m = new THREE.Mesh(new THREE.RingGeometry(0.17, 0.2, 40),
    new THREE.MeshBasicMaterial({ color: LEVEL_COLOR[level] || LEVEL_COLOR.wait, transparent: true, opacity: 0.85, side: THREE.DoubleSide, depthWrite: false }));
  m.rotation.x = -Math.PI / 2; m.position.y = 0.006; return m;
}

function makeLabel() {
  const el = document.createElement('div'); el.className = 'plate';
  el.innerHTML = '<span class="tag"></span><div class="nick"></div>' +
    '<div class="status"><span class="led"></span><span class="st"></span><span class="ms"></span></div><div class="prof"></div>';
  labels.append(el); return el;
}

// ---------- ficha da profissão (barras como no jogo) ----------
function profCard(p, extra) {
  const card = document.createElement('div'); card.className = 'pzcard';
  const h = document.createElement('h4'); h.textContent = p ? p.name : 'Sem profissão'; card.append(h);
  const sub = document.createElement('div'); sub.className = 'sub2';
  if (p) {
    const cost = document.createElement('span'); cost.className = 'cost' + (p.cost < 0 ? ' neg' : '');
    cost.textContent = `${p.cost > 0 ? '+' : ''}${p.cost} pontos`;
    sub.append(cost);
  }
  if (extra) sub.append((p ? ' · ' : '') + extra);
  card.append(sub);
  const boosts = (p && p.boosts) || [];
  if (!boosts.length) {
    const n = document.createElement('div'); n.className = 'none'; n.textContent = 'Nenhuma habilidade inicial'; card.append(n);
  }
  for (const b of boosts) {
    const row = document.createElement('div'); row.className = 'skill';
    const name = document.createElement('span'); name.textContent = b.name;
    const bars = document.createElement('span'); bars.className = 'bars';
    for (let i = 0; i < 10; i++) { const x = document.createElement('i'); if (i < b.level) x.className = 'on'; bars.append(x); }
    const lvl = document.createElement('span'); lvl.className = 'lvl'; lvl.textContent = '+' + b.level;
    row.append(name, bars, lvl); card.append(row);
  }
  if (p && p.traits && p.traits.length) {
    const t = document.createElement('div'); t.className = 'traits';
    const b = document.createElement('b'); b.textContent = 'Traços: ';
    t.append(b, p.traits.join(', ')); card.append(t);
  }
  return card;
}

window.pzProfCard = p => profCard(p);

// ---------- ficha ao passar o mouse sobre o personagem ----------
const tip = document.createElement('div'); tip.id = 'plate-tip'; tip.hidden = true; root.append(tip);
let tipFor = null, mouse = null;
const raycaster = new THREE.Raycaster(), ndc = new THREE.Vector2();
function entryAt(clientX, clientY) {
  const r = canvas.getBoundingClientRect();
  ndc.set(((clientX - r.left) / r.width) * 2 - 1, -((clientY - r.top) / r.height) * 2 + 1);
  raycaster.setFromCamera(ndc, camera);
  const hits = raycaster.intersectObjects([...players.values()].map(e => e.hit), false);
  return hits.length ? hits[0].object.userData.entry : null;
}
function showTip(e) {
  if (!e || !e.data.professionInfo) { hideTip(); return; } // só depois de escolher a profissão
  if (tipFor !== e) {
    const c = e.data.character;
    const who = c ? `${e.data.name} · ${c.name}` : e.data.name;
    tip.replaceChildren(profCard(e.data.professionInfo, who));
    tipFor = e;
  }
  tip.hidden = false; placeTip();
}
function hideTip() { tip.hidden = true; tipFor = null; canvas.style.cursor = ''; }
function placeTip() {
  if (!tipFor || !mouse) return;
  const base = root.getBoundingClientRect();
  const w = tip.offsetWidth, h = tip.offsetHeight;
  let x = mouse.x - base.left + 18, y = mouse.y - base.top + 14;
  if (x + w > base.width - 8) x = mouse.x - base.left - w - 18;
  if (y + h > base.height - 8) y = base.height - h - 8;
  tip.style.left = Math.max(8, x) + 'px'; tip.style.top = Math.max(8, y) + 'px';
}
canvas.addEventListener('pointermove', ev => {
  mouse = { x: ev.clientX, y: ev.clientY };
  const e = entryAt(ev.clientX, ev.clientY);
  canvas.style.cursor = e && e.data.professionInfo ? 'help' : '';
  if (e) showTip(e); else hideTip();
});
canvas.addEventListener('pointerleave', () => { mouse = null; hideTip(); });

function fillLabel(p, el) {
  const tag = p.host && p.self ? 'VOCÊ · ANFITRIÃO' : p.host ? 'ANFITRIÃO' : (p.self ? 'VOCÊ' : '');
  const tagEl = el.querySelector('.tag');
  tagEl.textContent = tag; tagEl.style.display = tag ? '' : 'none';
  tagEl.classList.toggle('host', !!p.host);
  el.querySelector('.nick').textContent = p.name;
  const st = el.querySelector('.status');
  st.classList.toggle('online', !!p.online);
  st.querySelector('.st').textContent = p.online ? 'online' : 'offline';
  const ms = st.querySelector('.ms');
  ms.className = 'ms ' + (p.level || 'wait');
  ms.textContent = !p.online ? '' : p.host ? '· servidor' : (p.pingMs ? '· ' + Math.round(p.pingMs) + ' ms' : '· …');
  el.querySelector('.prof').textContent = p.professionInfo ? p.professionInfo.name : 'Sem profissão';
}

function prettyOutfit(o) {
  return (o || '').replace(/_/g, ' ').replace(/([a-z])([A-Z])/g, '$1 $2').replace(/\d+$/, '').trim();
}

function addPlayer(p, slot) {
  const group = new THREE.Group();
  const [x, z] = SLOTS[slot % SLOTS.length];
  group.position.set(x, 0, z);
  group.rotation.y = -x * 0.18;
  group.scale.setScalar(0.001);
  scene.add(group);
  const entry = { group, mixer: null, data: p, label: makeLabel(), ring: ringFor(p.level), born: performance.now(), head: null, slot };
  group.add(entry.ring);
  entry.hit = new THREE.Mesh(new THREE.CylinderGeometry(0.2, 0.2, 1.0, 10), new THREE.MeshBasicMaterial({ visible: false }));
  entry.hit.position.y = 0.5; entry.hit.userData.entry = entry; group.add(entry.hit);
  fillLabel(p, entry.label);
  const key = playerKey(p);
  players.set(key, entry);
  gltfLoader.load(`/pz/char/${p.seed}?sex=${encodeURIComponent(p.sex || '')}&prof=${encodeURIComponent(p.profession || '')}`, gltf => {
    if (players.get(key) !== entry) return;
    const model = gltf.scene;
    model.traverse(o => {
      if (o.isMesh) { o.castShadow = true; o.frustumCulled = false; }
      if (o.isBone && o.name === 'Bip01_Head') entry.head = o;
    });
    group.add(model);
    if (gltf.animations.length) {
      entry.mixer = new THREE.AnimationMixer(model);
      const act = entry.mixer.clipAction(gltf.animations[0]);
      act.play(); act.time = Math.random() * gltf.animations[0].duration;
      entry.mixer.timeScale = idleSpeed * (0.9 + Math.random() * 0.2);
    }
  }, undefined, () => { entry.label.querySelector('.prof').textContent = 'personagem indisponível'; });
}

// a different sex or profession means a different model
function playerKey(p) { return `${p.seed}|${p.sex || ''}|${p.profession || ''}`; }

function removePlayer(seed) {
  const e = players.get(seed);
  if (!e) return;
  if (tipFor === e) hideTip();
  scene.remove(e.group); e.label.remove(); players.delete(seed);
}

function updatePlayers(list) {
  const seen = new Set();
  // você no centro; depois o anfitrião; depois os demais
  const order = [...list].sort((a, b) => (!!b.self - !!a.self) || (!!b.host - !!a.host) || a.name.localeCompare(b.name));
  order.forEach((p, i) => {
    const key = playerKey(p);
    seen.add(key);
    const e = players.get(key);
    if (!e) { addPlayer(p, i); return; }
    e.data = p; fillLabel(p, e.label);
    if (tipFor === e) { tipFor = null; showTip(e); }
    e.ring.material.color.setHex(LEVEL_COLOR[p.level] || LEVEL_COLOR.wait);
    if (e.slot !== i) {
      e.slot = i; const [x, z] = SLOTS[i % SLOTS.length];
      e.target = new THREE.Vector3(x, 0, z);
    }
  });
  for (const seed of [...players.keys()]) if (!seen.has(seed)) removePlayer(seed);
}

// ---------- laço ----------
const clock = new THREE.Clock();
const tmp = new THREE.Vector3();
function resize() {
  const w = root.clientWidth, h = root.clientHeight;
  if (canvas.width !== Math.floor(w * renderer.getPixelRatio()) || canvas.height !== Math.floor(h * renderer.getPixelRatio())) {
    renderer.setSize(w, h, false);
    camera.aspect = w / h;
    camera.fov = w < 600 ? 42 : 30;
    camera.updateProjectionMatrix();
  }
}
function frame() {
  const dt = Math.min(clock.getDelta(), 0.1), t = clock.elapsedTime;
  resize();
  camera.position.set(camBase.x + Math.sin(t * 0.15) * 0.08, camBase.y + Math.sin(t * 0.21) * 0.02, camBase.z);
  camera.lookAt(camTarget);

  fireLight.intensity = 2 + Math.sin(t * 11) * 0.25 + Math.sin(t * 17.3) * 0.2 + Math.random() * 0.15;
  flame.scale.set(1 + Math.sin(t * 9) * 0.08, 1 + Math.sin(t * 13) * 0.15, 1);
  for (let i = 0; i < SPARKS; i++) {
    sparkLife[i] += dt * (0.35 + (i % 7) * 0.05);
    if (sparkLife[i] > 1) sparkLife[i] = 0;
    const l = sparkLife[i];
    sparkPos[i * 3] = firePos.x + Math.sin(i * 12.9 + l * 6) * 0.08 * l;
    sparkPos[i * 3 + 1] = firePos.y + 0.1 + l * 0.9;
    sparkPos[i * 3 + 2] = firePos.z + Math.cos(i * 7.3 + l * 5) * 0.08 * l;
  }
  sparkGeo.attributes.position.needsUpdate = true;

  const rect = root.getBoundingClientRect();
  for (const e of players.values()) {
    const age = (performance.now() - e.born) / 600;
    const s = age >= 1 ? 1 : 1 - Math.pow(1 - age, 3);
    e.group.scale.setScalar(Math.max(0.001, s));
    if (e.target) { e.group.position.lerp(e.target, Math.min(1, dt * 4)); if (e.group.position.distanceTo(e.target) < 0.002) e.target = null; }
    if (e.mixer) e.mixer.update(dt);
    e.ring.material.opacity = 0.55 + Math.sin(t * 3 + e.slot) * 0.25;
    if (e.head) e.head.getWorldPosition(tmp); else tmp.set(e.group.position.x, 0.95, e.group.position.z);
    tmp.y += 0.2;
    const dist = camera.position.distanceTo(tmp);
    tmp.project(camera);
    const scale = Math.max(0.78, Math.min(1, 2.9 / dist));
    e.label.style.zIndex = String(100 - e.slot);
    const vis = tmp.z < 1;
    e.label.style.display = vis ? '' : 'none';
    e.label.style.transform = `translate(${(tmp.x * 0.5 + 0.5) * rect.width}px, ${(-tmp.y * 0.5 + 0.5) * rect.height}px) translate(-50%, -100%) scale(${scale.toFixed(3)})`;
  }
  placeTip();
  renderer.render(scene, camera);
  requestAnimationFrame(frame);
}

// ---------- dados ----------
let started = false;
async function refresh() {
  try {
    const demo = new URLSearchParams(location.search).get('demo');
    const st = await (await fetch('/api/lobby' + (demo ? '?demo=' + encodeURIComponent(demo) : ''))).json();
    if (!st.gameFound) {
      msg.textContent = 'Project Zomboid não encontrado neste PC. O lobby 3D usa os arquivos do seu próprio jogo.';
      msg.hidden = false;
    } else {
      msg.hidden = true;
      if (st.idleSpeed > 0) idleSpeed = st.idleSpeed;
      if (!started) { started = true; placeProps(); }
      updatePlayers(st.players || []);
    }
  } catch { /* PZLink fechado */ }
}
async function poll() { await refresh(); setTimeout(poll, 2000); }
window.pzLobbyRefresh = refresh;
// ?demo=N&tip=apelido abre a ficha de um jogador (para fotos e testes sem mouse)
const tipDemo = new URLSearchParams(location.search).get('tip');
if (tipDemo) setTimeout(() => {
  const e = [...players.values()].find(x => x.data.name === tipDemo);
  if (!e) return;
  const v = new THREE.Vector3(); e.hit.getWorldPosition(v); v.project(camera);
  const r = canvas.getBoundingClientRect();
  mouse = { x: r.left + (v.x * 0.5 + 0.5) * r.width, y: r.top + (-v.y * 0.5 + 0.5) * r.height };
  showTip(e);
}, 6000);
poll();
requestAnimationFrame(frame);
