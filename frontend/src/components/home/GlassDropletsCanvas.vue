<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';

interface Droplet {
  x: number;
  y: number;
  radius: number;
  isSliding: boolean;
  slideSpeed: number;
  opacity: number;
  trail: { x: number; y: number; r: number; alpha: number }[];
  lastSlideTime: number;
  slideInterval: number;
  mass: number;
  drift: number;
  wobble: number;
  trailTick: number;
}

interface SplashParticle {
  angle: number;
  distance: number;
  speed: number;
  size: number;
  alpha: number;
}

interface ImpactEvent {
  x: number;
  y: number;
  age: number;
  life: number;
  radius: number;
  particles: SplashParticle[];
  spawnedDrop: boolean;
}

interface GlassDropletsCanvasProps {
  enabled: boolean;
  animated?: boolean;
  quality?: 'standard' | 'balanced';
  mouseX?: number;
  mouseY?: number;
  zIndex?: number;
}

const props = withDefaults(defineProps<GlassDropletsCanvasProps>(), {
  animated: true,
  quality: 'standard',
  mouseX: 0,
  mouseY: 0,
  zIndex: 30,
});

const canvasRef = ref<HTMLCanvasElement | null>(null);
const mouseRef = { current: { x: props.mouseX, y: props.mouseY } };
let disposeAnimation: (() => void) | undefined;

function startAnimation() {
  disposeAnimation?.();
  disposeAnimation = undefined;

  if (typeof navigator !== 'undefined' && /jsdom/i.test(navigator.userAgent)) return;

  // Decorative motion should not retain a frame loop or global click handler
  // when the host has disabled animation.
  if (!props.enabled) return;

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

  const droplets: Droplet[] = [];
  const impacts: ImpactEvent[] = [];
  const staticLimit = props.quality === 'balanced' ? 24 : 72;
  const slidingLimit = props.quality === 'balanced' ? 0 : 12;
  const density = props.quality === 'balanced' ? 42000 : 21000;
  const maxDroplets = staticLimit + slidingLimit + 24;

  function addTransientDroplet(droplet: Droplet) {
    // Clicks and periodic impacts must not grow the collision loop indefinitely.
    if (droplets.length >= maxDroplets) droplets.splice(staticLimit + slidingLimit, 1);
    droplets.push(droplet);
  }

  function initDroplets() {
    droplets.length = 0;
    impacts.length = 0;
    const staticDropletsCount = Math.min(staticLimit, Math.floor((width * height) / density));
    const slidingDropletsCount = Math.min(slidingLimit, Math.floor((width * height) / 52000));

    // Static condensation micro-droplets on glass.
    for (let i = 0; i < staticDropletsCount; i++) {
      droplets.push({
        x: Math.random() * width,
        y: Math.random() * height,
        radius: 1 + Math.random() * 2.7,
        isSliding: false,
        slideSpeed: 0,
        opacity: 0.18 + Math.random() * 0.17,
        trail: [],
        lastSlideTime: Date.now(),
        slideInterval: 9999999,
        mass: 1,
        drift: 0,
        wobble: Math.random() * Math.PI * 2,
        trailTick: 0,
      });
    }

    // Sparsely sliding droplets with natural gravity.
    for (let i = 0; i < slidingDropletsCount; i++) {
      droplets.push({
        x: Math.random() * width,
        y: Math.random() * (height * 0.6),
        radius: 2.8 + Math.random() * 3.4,
        isSliding: false,
        slideSpeed: 0,
        opacity: 0.58 + Math.random() * 0.3,
        trail: [],
        lastSlideTime: Date.now() + Math.random() * 5000,
        slideInterval: 4000 + Math.random() * 8000,
        mass: 2 + Math.random() * 2,
        drift: (Math.random() - 0.5) * 0.24,
        wobble: Math.random() * Math.PI * 2,
        trailTick: 0,
      });
    }
  }

  const handleResize = () => {
    width = canvas.width = window.innerWidth;
    height = canvas.height = window.innerHeight;
    initDroplets();
    if (!props.animated) render();
  };

  window.addEventListener('resize', handleResize);
  initDroplets();

  let nextImpactAt = performance.now() + 700 + Math.random() * 1000;

  // Click on the page to spawn a gentle water droplet.
  const handleCanvasClick = (event: MouseEvent) => {
    const rect = canvas.getBoundingClientRect();
    const clickX = event.clientX - rect.left;
    const clickY = event.clientY - rect.top;

    addTransientDroplet({
      x: clickX,
      y: clickY,
      radius: 3 + Math.random() * 1.5,
      isSliding: true,
      slideSpeed: 0.8 + Math.random() * 0.8,
      opacity: 0.65,
      trail: [],
      lastSlideTime: Date.now(),
      slideInterval: 800,
      mass: 3,
      drift: (Math.random() - 0.5) * 0.3,
      wobble: Math.random() * Math.PI * 2,
      trailTick: 0,
    });
  };

  if (props.animated) {
    window.addEventListener('click', handleCanvasClick);
  }

  // Draw one realistic water droplet with refraction highlight and soft meniscus.
  function drawDroplet(
    context: CanvasRenderingContext2D,
    x: number,
    y: number,
    radius: number,
    alpha: number,
    stretch = 1,
  ) {
    if (radius <= 0) return;

    context.save();
    context.beginPath();
    // A subtly asymmetric meniscus reads as water sitting on glass.
    context.moveTo(x + radius * 0.82, y);
    context.bezierCurveTo(
      x + radius * 0.94,
      y + radius * 0.42 * stretch,
      x + radius * 0.36,
      y + radius * 1.02 * stretch,
      x,
      y + radius * stretch,
    );
    context.bezierCurveTo(
      x - radius * 0.58,
      y + radius * 0.93 * stretch,
      x - radius * 0.98,
      y + radius * 0.34 * stretch,
      x - radius * 0.8,
      y - radius * 0.06 * stretch,
    );
    context.bezierCurveTo(
      x - radius * 0.62,
      y - radius * 0.72 * stretch,
      x + radius * 0.2,
      y - radius * 0.96 * stretch,
      x + radius * 0.82,
      y,
    );
    context.closePath();

    // Edge shadow on glass.
    context.strokeStyle = `rgba(10, 18, 30, ${alpha * 0.3})`;
    context.lineWidth = Math.max(0.5, radius * 0.15);
    context.stroke();

    // Refractive glass fill.
    const gradient = context.createRadialGradient(
      x - radius * 0.25,
      y - radius * 0.25,
      radius * 0.1,
      x,
      y,
      radius,
    );
    gradient.addColorStop(0, `rgba(255, 255, 255, ${alpha * 0.4})`);
    gradient.addColorStop(0.6, `rgba(180, 215, 245, ${alpha * 0.12})`);
    gradient.addColorStop(1, `rgba(8, 15, 28, ${alpha * 0.35})`);
    context.fillStyle = gradient;
    context.fill();

    // Specular highlight dot.
    context.beginPath();
    context.arc(
      x - radius * 0.32,
      y - radius * 0.32,
      Math.max(0.6, radius * 0.25),
      0,
      Math.PI * 2,
    );
    context.fillStyle = `rgba(255, 255, 255, ${Math.min(1, alpha * 0.85)})`;
    context.fill();

    // A narrow lower reflection suggests thickness and refraction.
    context.beginPath();
    context.ellipse(
      x + radius * 0.18,
      y + radius * 0.5 * stretch,
      radius * 0.34,
      Math.max(0.35, radius * 0.09),
      -0.12,
      0,
      Math.PI * 2,
    );
    context.fillStyle = `rgba(225, 240, 255, ${alpha * 0.24})`;
    context.fill();

    context.restore();
  }

  // A raindrop striking the upper edge briefly fans out before gravity pulls
  // the pooled water into a single sliding drop.
  function spawnImpact(time: number) {
    const edgeInset = Math.min(width * 0.08, 150);
    const impact: ImpactEvent = {
      x: edgeInset + Math.random() * Math.max(1, width - edgeInset * 2),
      y: Math.max(30, height * 0.035) + Math.random() * 10,
      age: 0,
      life: 0.9 + Math.random() * 0.3,
      radius: 2.5 + Math.random() * 2.2,
      particles: [],
      spawnedDrop: false,
    };

    const particleCount = 4 + Math.floor(Math.random() * 4);
    for (let i = 0; i < particleCount; i++) {
      // Mostly lateral spray, with two particles allowed to lift slightly.
      const angle = (Math.random() - 0.5) * Math.PI * 1.18;
      impact.particles.push({
        angle,
        distance: 0,
        speed: 28 + Math.random() * 42,
        size: 0.9 + Math.random() * 1.5,
        alpha: 0.35 + Math.random() * 0.3,
      });
    }
    impacts.push(impact);
    nextImpactAt = time + 1200 + Math.random() * 2600;
  }

  function drawImpact(context: CanvasRenderingContext2D, impact: ImpactEvent, pX: number, pY: number, delta: number) {
    const progress = impact.age / impact.life;
    const fade = Math.max(0, 1 - progress);
    const x = impact.x + pX;
    const y = impact.y + pY;

    context.save();
    context.lineCap = 'round';
    // A shallow expanding ring makes the impact read on the pane surface.
    const ringRadius = impact.radius + progress * 20;
    context.beginPath();
    context.ellipse(x, y, ringRadius * 2.35, Math.max(1.2, ringRadius * 0.5), 0, 0, Math.PI * 2);
    context.strokeStyle = `rgba(220, 244, 255, ${fade * 0.78})`;
    context.lineWidth = Math.max(0.8, 1.5 * fade);
    context.stroke();

    // Fine outward droplets fade much faster than the pooled drop.
    for (const particle of impact.particles) {
      particle.distance += particle.speed * delta;
      const px = x + Math.cos(particle.angle) * particle.distance;
      const py = y + Math.sin(particle.angle) * particle.distance * 0.42;
      context.beginPath();
      context.arc(px, py, particle.size * (0.75 + fade * 0.3), 0, Math.PI * 2);
      context.fillStyle = `rgba(221, 245, 255, ${particle.alpha * fade})`;
      context.fill();
    }

    // Bright wet contact point at the centre of the splash.
    context.beginPath();
    context.ellipse(x, y + 1, impact.radius * 1.45, impact.radius * 0.42, 0, 0, Math.PI * 2);
    context.fillStyle = `rgba(231, 249, 255, ${fade * 0.36})`;
    context.fill();
    context.restore();
  }

  let lastFrame = performance.now();
  let lastPaint = lastFrame;
  const frameInterval = props.quality === 'balanced' ? 1000 / 30 : 1000 / 60;
  const render = () => {
    const frameNow = performance.now();
    const elapsed = frameNow - lastFrame;
    if (props.animated && elapsed + 0.5 < frameInterval) {
      animId = requestFrame(render);
      return;
    }
    const delta = props.animated ? Math.min(0.1, Math.max(0, (frameNow - lastPaint) / 1000)) : 0;
    lastPaint = frameNow;
    lastFrame = frameNow - (elapsed >= frameInterval ? elapsed % frameInterval : 0);
    ctx.clearRect(0, 0, width, height);

    if (!props.enabled) {
      animId = requestFrame(render);
      return;
    }

    const now = Date.now();
    const frameTime = performance.now();
    // Very gentle parallax offset on droplets.
    const pX = props.animated ? (mouseRef.current.x / (width || 1) - 0.5) * 5 : 0;
    const pY = props.animated ? (mouseRef.current.y / (height || 1) - 0.5) * 5 : 0;

    if (props.animated && frameTime >= nextImpactAt) spawnImpact(frameTime);

    // Update and draw impact splashes before the persistent droplet field.
    for (let i = impacts.length - 1; i >= 0; i--) {
      const impact = impacts[i];
      impact.age += delta;
      if (!impact.spawnedDrop && impact.age >= 0.12) {
        impact.spawnedDrop = true;
        addTransientDroplet({
          x: impact.x + (Math.random() - 0.5) * 3,
          y: impact.y + 3,
          radius: 3.1 + Math.random() * 1.8,
          isSliding: true,
          slideSpeed: 15 + Math.random() * 13,
          opacity: 0.62 + Math.random() * 0.2,
          trail: [],
          lastSlideTime: now,
          slideInterval: 900,
          mass: 2.5 + Math.random() * 1.7,
          drift: (Math.random() - 0.5) * 0.24,
          wobble: Math.random() * Math.PI * 2,
          trailTick: 0,
        });
      }
      drawImpact(ctx, impact, pX, pY, delta);
      if (impact.age >= impact.life) impacts.splice(i, 1);
    }

    for (let i = 0; i < droplets.length; i++) {
      const droplet = droplets[i];

      // Trigger slide if interval elapsed.
      if (!droplet.isSliding && now - droplet.lastSlideTime > droplet.slideInterval) {
        droplet.isSliding = true;
        droplet.slideSpeed = 10 + Math.random() * 18;
        droplet.lastSlideTime = now;
      }

      // Process sliding physics.
      if (droplet.isSliding) {
        // Preserve a short, curved history and draw it as a soft wet track.
        droplet.trailTick += delta;
        if (droplet.trailTick > 0.035) {
          droplet.trailTick = 0;
          droplet.trail.push({
            x: droplet.x,
            y: droplet.y,
            r: droplet.radius,
            alpha: droplet.opacity,
          });
          if (droplet.trail.length > 18) droplet.trail.shift();
        }

        droplet.y += droplet.slideSpeed * delta;
        // Glass texture nudges the drop into a gentle, non-linear path.
        droplet.wobble += delta * (0.8 + droplet.mass * 0.12);
        droplet.x += (droplet.drift + Math.sin(droplet.wobble) * 0.18) * droplet.slideSpeed * delta;

        // Merge with nearby static droplets when colliding.
        for (let j = 0; j < droplets.length; j++) {
          if (i !== j && !droplets[j].isSliding) {
            const target = droplets[j];
            const distance = Math.hypot(droplet.x - target.x, droplet.y - target.y);
            if (distance < droplet.radius + target.radius) {
              droplet.radius = Math.min(5.5, Math.sqrt(droplet.radius ** 2 + target.radius ** 2 * 0.5));
              droplet.slideSpeed = Math.min(75, droplet.slideSpeed + 8);
              // Respawn the absorbed static droplet elsewhere.
              target.x = Math.random() * width;
              target.y = Math.random() * height;
              target.radius = 1.2 + Math.random() * 2.5;
            }
          }
        }

        // Very gradual acceleration downwards.
        droplet.slideSpeed = Math.min(88, droplet.slideSpeed + 8 * delta);

        // If reached bottom, recycle to top.
        if (droplet.y > height + 20) {
          droplet.y = -droplet.radius - Math.random() * 30;
          droplet.x = Math.random() * width;
          droplet.radius = 2.8 + Math.random() * 2;
          droplet.isSliding = false;
          droplet.slideSpeed = 0;
          droplet.trail = [];
          droplet.lastSlideTime = now;
          droplet.slideInterval = 5000 + Math.random() * 10000;
        }
      }

      // Draw the wet trail from oldest to newest, tapering toward the drop.
      if (droplet.isSliding && droplet.trail.length > 1) {
        ctx.save();
        ctx.lineCap = 'round';
        ctx.beginPath();
        for (let t = 0; t < droplet.trail.length; t++) {
          const trailPoint = droplet.trail[t];
          if (t === 0) ctx.moveTo(trailPoint.x + pX, trailPoint.y + pY);
          else ctx.lineTo(trailPoint.x + pX, trailPoint.y + pY);
        }
        const trailGradient = ctx.createLinearGradient(0, droplet.trail[0].y, 0, droplet.y);
        trailGradient.addColorStop(0, 'rgba(185, 222, 250, 0)');
        trailGradient.addColorStop(0.55, `rgba(185, 222, 250, ${droplet.opacity * 0.11})`);
        trailGradient.addColorStop(1, `rgba(239, 249, 255, ${droplet.opacity * 0.38})`);
        ctx.strokeStyle = trailGradient;
        ctx.lineWidth = Math.max(0.7, droplet.radius * 0.26);
        ctx.stroke();
        ctx.restore();
      }

      drawDroplet(ctx, droplet.x + pX, droplet.y + pY, droplet.radius, droplet.opacity, droplet.isSliding ? 1.45 : 1);
    }

    if (props.animated) {
      animId = requestFrame(render);
    }
  };

  render();
  disposeAnimation = () => {
    cancelFrame(animId);
    window.removeEventListener('resize', handleResize);
    window.removeEventListener('click', handleCanvasClick);
  };
}

onMounted(startAnimation);
watch(() => [props.enabled, props.animated, props.quality], startAnimation);
watch([() => props.mouseX, () => props.mouseY], () => {
  mouseRef.current = { x: props.mouseX, y: props.mouseY };
});
onBeforeUnmount(() => {
  disposeAnimation?.();
  disposeAnimation = undefined;
});
</script>

<template>
  <Teleport to="body">
    <canvas
      ref="canvasRef"
      class="pointer-events-none fixed inset-0 h-full w-full mix-blend-screen"
      :style="{ zIndex: props.zIndex, opacity: props.enabled ? 0.92 : 0, transition: 'opacity 0.8s ease' }"
    />
  </Teleport>
</template>
