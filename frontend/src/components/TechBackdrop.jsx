import React, { useEffect, useRef } from 'react';

const palette = [
  [77, 226, 255],
  [157, 107, 255],
  [255, 102, 194],
];

export default function TechBackdrop() {
  const canvasRef = useRef(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    const context = canvas?.getContext('2d');
    if (!canvas || !context) return undefined;

    const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    let width = 0;
    let height = 0;
    let frame = 0;
    let particles = [];
    let lastFrame = 0;
    const pointer = { x: -1000, y: -1000, active: false };

    const createParticle = () => {
      const depth = 0.35 + Math.random() * 0.65;
      const x = Math.random() * width;
      const y = Math.random() * height;
      return {
        x,
        y,
        previousX: x,
        previousY: y,
        vx: (Math.random() - 0.5) * (0.15 + depth * 0.24),
        vy: (Math.random() - 0.5) * (0.15 + depth * 0.24),
        radius: 0.55 + depth * 1.25,
        depth,
        color: palette[Math.floor(Math.random() * palette.length)],
        phase: Math.random() * Math.PI * 2,
        beacon: Math.random() < 0.13,
      };
    };

    const resize = () => {
      const ratio = Math.min(window.devicePixelRatio || 1, 2);
      width = window.innerWidth;
      height = window.innerHeight;
      canvas.width = Math.round(width * ratio);
      canvas.height = Math.round(height * ratio);
      canvas.style.width = `${width}px`;
      canvas.style.height = `${height}px`;
      context.setTransform(ratio, 0, 0, ratio, 0, 0);
      const count = width < 700 ? 27 : Math.min(68, Math.round((width * height) / 22000));
      particles = Array.from({ length: count }, createParticle);
    };

    const draw = (timestamp = 0) => {
      if (!reducedMotion && timestamp - lastFrame < 28) {
        frame = window.requestAnimationFrame(draw);
        return;
      }
      lastFrame = timestamp;
      context.clearRect(0, 0, width, height);
      context.globalCompositeOperation = 'lighter';

      for (let index = 0; index < particles.length; index += 1) {
        const particle = particles[index];
        if (!reducedMotion) {
          particle.previousX = particle.x;
          particle.previousY = particle.y;
          const dx = pointer.x - particle.x;
          const dy = pointer.y - particle.y;
          const pointerDistance = Math.hypot(dx, dy);
          if (pointer.active && pointerDistance < 175 && pointerDistance > 1) {
            const force = (1 - pointerDistance / 175) * 0.012 * particle.depth;
            particle.vx -= (dx / pointerDistance) * force;
            particle.vy -= (dy / pointerDistance) * force;
          }
          particle.vx += Math.sin(timestamp * 0.00013 + particle.phase) * 0.00045;
          particle.vy += Math.cos(timestamp * 0.00011 + particle.phase) * 0.00045;
          particle.vx *= 0.996;
          particle.vy *= 0.996;
          particle.x += particle.vx;
          particle.y += particle.vy;
          if (particle.x < -12) particle.x = particle.previousX = width + 12;
          if (particle.x > width + 12) particle.x = particle.previousX = -12;
          if (particle.y < -12) particle.y = particle.previousY = height + 12;
          if (particle.y > height + 12) particle.y = particle.previousY = -12;
        }

        for (let nextIndex = index + 1; nextIndex < particles.length; nextIndex += 1) {
          const next = particles[nextIndex];
          const distance = Math.hypot(particle.x - next.x, particle.y - next.y);
          const connectionRange = 105 + Math.min(particle.depth, next.depth) * 66;
          if (distance < connectionRange) {
            const alpha = (1 - distance / connectionRange) * (0.045 + Math.min(particle.depth, next.depth) * 0.12);
            const red = Math.round((particle.color[0] + next.color[0]) / 2);
            const green = Math.round((particle.color[1] + next.color[1]) / 2);
            const blue = Math.round((particle.color[2] + next.color[2]) / 2);
            context.beginPath();
            context.moveTo(particle.x, particle.y);
            context.lineTo(next.x, next.y);
            context.strokeStyle = `rgba(${red}, ${green}, ${blue}, ${alpha})`;
            context.lineWidth = 0.35 + Math.min(particle.depth, next.depth) * 0.45;
            context.stroke();

            if ((index * 11 + nextIndex * 7) % 29 === 0) {
              const progress = (timestamp * 0.00012 + particle.phase / (Math.PI * 2)) % 1;
              const pulseX = particle.x + (next.x - particle.x) * progress;
              const pulseY = particle.y + (next.y - particle.y) * progress;
              context.beginPath();
              context.arc(pulseX, pulseY, 0.65 + particle.depth * 0.8, 0, Math.PI * 2);
              context.fillStyle = `rgba(${red}, ${green}, ${blue}, ${Math.min(alpha * 4, 0.76)})`;
              context.shadowColor = `rgba(${red}, ${green}, ${blue}, .72)`;
              context.shadowBlur = 9;
              context.fill();
              context.shadowBlur = 0;
            }
          }
        }

        const pulse = reducedMotion ? 1 : 0.82 + Math.sin(timestamp * 0.0012 + particle.phase) * 0.18;
        const [red, green, blue] = particle.color;
        context.beginPath();
        context.moveTo(particle.x - particle.vx * 13, particle.y - particle.vy * 13);
        context.lineTo(particle.x, particle.y);
        context.strokeStyle = `rgba(${red}, ${green}, ${blue}, ${0.11 + particle.depth * 0.13})`;
        context.lineWidth = 0.45 + particle.depth * 0.65;
        context.stroke();

        if (particle.beacon) {
          const ringPulse = 3.4 + (Math.sin(timestamp * 0.001 + particle.phase) + 1) * 1.6;
          context.beginPath();
          context.arc(particle.x, particle.y, particle.radius * ringPulse, 0, Math.PI * 2);
          context.strokeStyle = `rgba(${red}, ${green}, ${blue}, ${0.045 + particle.depth * 0.055})`;
          context.lineWidth = 0.55;
          context.stroke();
        }

        context.beginPath();
        context.arc(particle.x, particle.y, particle.radius * pulse, 0, Math.PI * 2);
        context.fillStyle = `rgba(${red}, ${green}, ${blue}, ${0.42 + particle.depth * 0.46})`;
        context.shadowColor = `rgba(${red}, ${green}, ${blue}, 0.55)`;
        context.shadowBlur = 6 + particle.depth * 10;
        context.fill();
        context.shadowBlur = 0;
      }

      if (pointer.active && !reducedMotion) {
        particles.forEach((particle) => {
          const distance = Math.hypot(pointer.x - particle.x, pointer.y - particle.y);
          if (distance >= 190) return;
          const alpha = (1 - distance / 190) * 0.14 * particle.depth;
          context.beginPath();
          context.moveTo(pointer.x, pointer.y);
          context.lineTo(particle.x, particle.y);
          context.strokeStyle = `rgba(104, 226, 255, ${alpha})`;
          context.lineWidth = 0.6;
          context.stroke();
        });
      }

      context.globalCompositeOperation = 'source-over';

      if (!reducedMotion) frame = window.requestAnimationFrame(draw);
    };

    const updatePointer = (event) => { pointer.x = event.clientX; pointer.y = event.clientY; pointer.active = true; };
    const clearPointer = () => { pointer.x = -1000; pointer.y = -1000; pointer.active = false; };
    resize();
    draw();
    window.addEventListener('resize', resize);
    window.addEventListener('pointermove', updatePointer, { passive: true });
    window.addEventListener('pointerleave', clearPointer);

    return () => {
      window.cancelAnimationFrame(frame);
      window.removeEventListener('resize', resize);
      window.removeEventListener('pointermove', updatePointer);
      window.removeEventListener('pointerleave', clearPointer);
    };
  }, []);

  return (
    <div className="tech-backdrop" aria-hidden="true">
      <canvas ref={canvasRef} />
      <div className="tech-grid" />
      <div className="tech-scan" />
      <div className="tech-vignette" />
    </div>
  );
}
