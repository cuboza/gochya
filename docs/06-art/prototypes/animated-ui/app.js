const characters = {
  fire:  { name: 'Рэй',  species: 'степной терран', asset: 'assets/fire.png',  accent: '#ff6b35', rgb: '255,107,53' },
  water: { name: 'Нери', species: 'приливный аксол', asset: 'assets/water.png', accent: '#4ecdc4', rgb: '78,205,196' },
  earth: { name: 'Торр', species: 'панцирный барс', asset: 'assets/earth.png', accent: '#c9a66b', rgb: '201,166,107' },
  steam: { name: 'Миро', species: 'тёплый мустелид', asset: 'assets/steam.png', accent: '#d8d6e8', rgb: '216,214,232' }
};

const needs = { food: 74, rest: 61, clean: 88, mood: 82 };
const reactions = { food: 'is-happy', clean: 'is-clean', mood: 'is-happy', rest: 'is-sleepy' };
const messages = { food: 'Отличный перекус · Еда +8', clean: 'Свежесть · Чистота +8', mood: 'Хорошая игра · Настрой +8', rest: 'Короткий отдых · Сон +8' };

function setCharacter(key) {
  const character = characters[key];
  document.body.dataset.element = key;
  document.documentElement.style.setProperty('--accent', character.accent);
  document.documentElement.style.setProperty('--accent-rgb', character.rgb);
  document.querySelectorAll('[data-creature]').forEach((image) => {
    image.src = character.asset;
    image.alt = `Игровой питомец ${key}`;
  });
  document.querySelectorAll('[data-pet-name]').forEach((node) => node.textContent = character.name);
  document.querySelectorAll('[data-pet-species]').forEach((node) => node.textContent = character.species);
  document.querySelector('[data-feedback]').textContent = `${character.name} готов к приключениям`;
  document.querySelectorAll('[data-select]').forEach((button) => button.classList.toggle('is-active', button.dataset.select === key));
  react('is-happy', 10);
}

function react(className, particleCount = 7) {
  document.querySelectorAll('.creature-wrap').forEach((wrap) => {
    wrap.classList.remove('is-happy', 'is-sleepy', 'is-clean', 'is-dojo');
    void wrap.offsetWidth;
    wrap.classList.add(className);
    wrap.addEventListener('animationend', () => wrap.classList.remove(className), { once: true });
  });
  document.querySelectorAll('.particle-field').forEach((field) => burst(field, particleCount));
}

function burst(field, count) {
  for (let index = 0; index < count; index += 1) {
    const particle = document.createElement('i');
    particle.className = 'particle';
    particle.style.left = `${28 + Math.random() * 44}%`;
    particle.style.top = `${42 + Math.random() * 28}%`;
    particle.style.setProperty('--drift', `${-34 + Math.random() * 68}px`);
    particle.style.setProperty('--duration', `${1.2 + Math.random() * 1.2}s`);
    field.append(particle);
    particle.addEventListener('animationend', () => particle.remove(), { once: true });
  }
}

function updateNeed(key) {
  needs[key] = Math.min(100, needs[key] + 8);
  const node = document.querySelector(`[data-need="${key}"]`);
  node.style.setProperty('--value', needs[key]);
  node.querySelector('b').textContent = `${needs[key]}%`;
  document.querySelector('[data-feedback]').textContent = messages[key];
  react(reactions[key]);
}

document.querySelectorAll('[data-select]').forEach((button) => button.addEventListener('click', () => setCharacter(button.dataset.select)));
document.querySelectorAll('[data-action]').forEach((button) => button.addEventListener('click', () => updateNeed(button.dataset.action)));
document.querySelectorAll('.nav-item').forEach((button) => button.addEventListener('click', () => {
  document.querySelectorAll('.nav-item').forEach((item) => item.classList.remove('is-active'));
  button.classList.add('is-active');
}));

document.querySelector('[data-watch-action="dojo"]').addEventListener('click', () => {
  document.querySelector('[data-watch-feedback]').textContent = 'Удар записан · Precision 86';
  document.querySelector('[data-feedback]').textContent = 'Dojo · новая техника сохранена';
  react('is-dojo', 16);
});
document.querySelector('[data-watch-action="care"]').addEventListener('click', () => updateNeed('clean'));
document.querySelector('[data-watch-action="pvp"]').addEventListener('click', () => {
  document.querySelector('[data-watch-feedback]').textContent = 'Соперник найден';
  react('is-dojo', 10);
});

setInterval(() => {
  const visibleFields = document.querySelectorAll('.particle-field');
  visibleFields.forEach((field) => Math.random() > .45 && burst(field, 1));
}, 1600);
