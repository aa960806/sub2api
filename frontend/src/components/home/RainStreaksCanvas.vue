<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';

interface RainStreaksCanvasProps {
  enabled: boolean;
  intensity?: number;
  windSpeed?: number;
}

interface RainStreak {
  x: number;
  y: number;
  length: number;
  speed: number;
  opacity: number;
  thickness: number;
  depth: number;
  angle: number;
  phase: number;
  layer: 'far' | 'mid' | 'near';
  warm: boolean;
}

const props = withDefaults(defineProps<RainStreaksCanvasProps>(), {
  intensity: 0.6,
  windSpeed: 3.5,
});

const canvasRef = ref<HTMLCanvasElement | null>(null);
let disposeAnimation: (() => void) | undefined;

function startAnimation() {
  disposeAnimation?.();
  disposeAnimation = undefined;

  // jsdom intentionally does not implement a 2D canvas. Skip the visual
  // effect there so component tests remain quiet while browsers use the
  // target animation unchanged.
  if (typeof navigator !== 'undefined' && /jsdom/i.test(navigator.userAgent)) return;

  const canvas = canvasRef.value;
  if (!canvas) return;
  let ctx: CanvasRenderingContext2D | null;
  try {
    ctx = canvas.getContext('2d');
  } catch {
    // Canvas is optional in SSR and DOM-only test environments.
    return;
  }
  if (!ctx) return;

  const requestFrame = typeof window.requestAnimationFrame === 'function'
    ? (callback: (timestamp: number) => void) => window.requestAnimationFrame(callback)
    : (callback: (timestamp: number) => void) => window.setTimeout(() => callback(performance.now()), 16);
  const cancelFrame = typeof window.cancelAnimationFrame === 'function'
    ? (id: number) => window.cancelAnimationFrame(id)
    : (id: number) => window.clearTimeout(id);

  let animId = 0;
  let width = (canvas.width = window.innerWidth);
  let height = (canvas.height = window.innerHeight);

  const handleResize = () => {
    width = canvas.width = window.innerWidth;
    height = canvas.height = window.innerHeight;
  };

  window.addEventListener('resize', handleResize);

  // Keep enough drops on screen for a cinematic curtain, while reserving a
  // smaller near-camera layer for the thick, luminous streaks.
  const count = Math.min(420, Math.floor(width * height * 0.0002 * (0.78 + props.intensity * 1.08)));
  const streaks: RainStreak[] = [];

  // Wind angle calculation: subtle slant.
  const windAngle = Math.min(0.32, Math.max(-0.32, (props.windSpeed - 2.5) * 0.04 + 0.14));

  for (let i = 0; i < count; i++) {
    const depth = 0.18 + Math.random() * 0.82;
    const layer = depth > 0.77 ? 'near' : depth > 0.43 ? 'mid' : 'far';
    const near = layer === 'near';
    const far = layer === 'far';
    streaks.push({
      x: Math.random() * (width + 200) - 100,
      y: Math.random() * height,
      length: (far ? 14 : near ? 112 : 42) + depth * (far ? 26 : near ? 100 : 58) + Math.random() * (near ? 60 : 30),
      speed: (far ? 92 : near ? 300 : 155) + depth * 250 + Math.random() * (near ? 170 : 100),
      opacity: (far ? 0.045 : near ? 0.12 : 0.075) + depth * (far ? 0.1 : near ? 0.28 : 0.24) + Math.random() * 0.09,
      thickness: (far ? 0.28 : near ? 1.55 : 0.62) + depth * (far ? 0.55 : near ? 2 : 1.2) + Math.random() * (near ? 1 : 0.5),
      depth,
      angle: windAngle + (Math.random() - 0.5) * (0.06 + (1 - depth) * 0.11),
      phase: Math.random() * Math.PI * 2,
      layer,
      // Occasional warm reflections sell the feeling of city lights caught
      // in wet glass without tinting the whole rain field.
      warm: Math.random() < (near ? 0.16 : 0.08),
    });
  }

  let lastFrame = performance.now();
  const render = () => {
    const now = performance.now();
    const delta = Math.min(0.035, (now - lastFrame) / 1000);
    lastFrame = now;
    ctx.clearRect(0, 0, width, height);

    if (props.enabled) {
      ctx.lineCap = 'round';
      ctx.globalCompositeOperation = 'lighter';

      for (let i = 0; i < streaks.length; i++) {
        const s = streaks[i];
        const dx = Math.sin(s.angle) * s.length;
        const dy = Math.cos(s.angle) * s.length;

        const pulse = 0.78 + Math.sin(now * 0.0014 + s.phase) * 0.15;
        // Rain travels from the upper tail to the lower drop head. Keep the
        // tail subdued and let the falling end catch the light.
        const body = ctx.createLinearGradient(s.x, s.y, s.x + dx, s.y + dy);
        const coolHead = s.warm ? '255, 226, 184' : '231, 249, 255';
        const coolBody = s.warm ? '245, 183, 126' : '126, 204, 250';
        body.addColorStop(0, `rgba(${coolBody}, ${s.opacity * 0.035})`);
        body.addColorStop(0.2, `rgba(${coolBody}, ${s.opacity * 0.11})`);
        body.addColorStop(0.62, `rgba(${coolBody}, ${s.opacity * 0.28})`);
        body.addColorStop(0.86, `rgba(${coolHead}, ${s.opacity * 0.62 * pulse})`);
        body.addColorStop(1, `rgba(${coolHead}, ${s.opacity * (s.layer === 'near' ? 0.9 : 0.7) * pulse})`);

        // A broad, faint halo gives the near layer a soft bloom.
        if (s.layer !== 'far') {
          ctx.beginPath();
          ctx.lineWidth = s.thickness * (s.layer === 'near' ? 3.6 : 2.4);
          ctx.strokeStyle = s.warm
            ? `rgba(255, 190, 118, ${s.opacity * 0.055})`
            : `rgba(104, 209, 255, ${s.opacity * 0.085})`;
          ctx.moveTo(s.x, s.y);
          ctx.lineTo(s.x + dx * 0.88, s.y + dy * 0.88);
          ctx.stroke();
        }

        ctx.beginPath();
        ctx.lineWidth = s.thickness;
        ctx.strokeStyle = body;
        ctx.moveTo(s.x, s.y);
        ctx.lineTo(s.x + dx, s.y + dy);
        ctx.stroke();

        // Short specular head: a drop catches a streetlight for a frame.
        const shimmer = (0.68 + Math.sin(now * 0.0012 + s.phase) * 0.16) * (s.layer === 'far' ? 0.62 : 1);
        ctx.beginPath();
        ctx.lineWidth = Math.max(0.45, s.thickness * (s.layer === 'near' ? 0.62 : 0.72));
        ctx.strokeStyle = s.warm
          ? `rgba(255, 224, 182, ${s.opacity * shimmer * 0.76})`
          : `rgba(246, 252, 255, ${s.opacity * shimmer})`;
        const headLength = Math.min(7 + s.depth * 6, s.length * 0.2);
        const headX = s.x + dx;
        const headY = s.y + dy;
        ctx.moveTo(headX - Math.sin(s.angle) * headLength, headY - Math.cos(s.angle) * headLength);
        ctx.lineTo(headX, headY);
        ctx.stroke();

        s.y += s.speed * delta;
        s.x += Math.sin(s.angle) * s.speed * delta;

        if (s.y > height + 20) {
          s.y = -s.length - Math.random() * 80;
          s.x = Math.random() * (width + 200) - 100;
        }
      }
      ctx.globalCompositeOperation = 'source-over';
    }

    animId = requestFrame(render);
  };

  render();
  disposeAnimation = () => {
    cancelFrame(animId);
    window.removeEventListener('resize', handleResize);
  };
}

onMounted(startAnimation);
watch(() => [props.enabled, props.intensity, props.windSpeed], startAnimation);
onBeforeUnmount(() => {
  disposeAnimation?.();
  disposeAnimation = undefined;
});
</script>

<template>
  <canvas
    ref="canvasRef"
    class="pointer-events-none absolute inset-0 z-0 h-full w-full mix-blend-screen"
    :style="{ opacity: props.enabled ? 0.82 : 0, transition: 'opacity 0.8s ease' }"
  />
</template>
