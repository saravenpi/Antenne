<script lang="ts">
	import { onDestroy } from 'svelte';

	// A single canvas visualizer used by both the régie mic and the listener
	// stream. It draws real spectrum data from an AnalyserNode when available,
	// smooths it for a clean look, and — when `fallback` is set — animates a
	// synthesized spectrum for browsers that don't expose analyser data for
	// HLS/MSE media elements (Firefox for Android, iOS Safari). When there's
	// neither an analyser nor fallback it idles (cleared).
	let {
		analyser = null,
		variant = 'bars',
		color = 'rgba(250,250,250,0.92)',
		fallback = false,
		class: className = ''
	}: {
		analyser?: AnalyserNode | null;
		variant?: 'bars' | 'radial';
		color?: string;
		fallback?: boolean;
		class?: string;
	} = $props();

	const COUNT = 56;
	let canvas: HTMLCanvasElement | undefined = $state();
	let raf = 0;
	let freq: Uint8Array<ArrayBuffer> | null = null;
	const level = new Float32Array(COUNT); // smoothed magnitudes, 0..1
	const target = new Float32Array(COUNT); // per-frame targets, 0..1

	function dims(cv: HTMLCanvasElement) {
		const dpr = Math.min(window.devicePixelRatio || 1, 2);
		const w = cv.clientWidth;
		const h = cv.clientHeight;
		const pw = Math.round(w * dpr);
		const ph = Math.round(h * dpr);
		if (cv.width !== pw || cv.height !== ph) {
			cv.width = pw;
			cv.height = ph;
		}
		return { w, h, dpr };
	}

	// Fill `out` from real analyser data. Returns false when the analyser is
	// absent or flat (no real signal), so the caller can fall back.
	function realSample(out: Float32Array): boolean {
		if (!analyser || !freq) return false;
		analyser.getByteFrequencyData(freq);
		const n = freq.length;
		let peak = 0;
		for (let i = 0; i < COUNT; i++) {
			// Weight toward the lower-mid range where voice/music energy sits.
			const idx = Math.min(n - 1, Math.floor(Math.pow(i / COUNT, 1.35) * n * 0.72));
			out[i] = freq[idx] / 255;
			if (freq[idx] > peak) peak = freq[idx];
		}
		return peak > 3;
	}

	// Time-driven synthetic spectrum, tapered at the edges so it reads as a
	// lively equalizer rather than a flat block.
	function synthSample(out: Float32Array, t: number) {
		for (let i = 0; i < COUNT; i++) {
			const a = Math.sin(t * 0.0022 + i * 0.3) * 0.5 + 0.5;
			const b = Math.sin(t * 0.006 + i * 0.11) * 0.5 + 0.5;
			const taper = 0.35 + 0.65 * Math.sin((i / COUNT) * Math.PI);
			out[i] = (a * 0.6 + b * 0.4) * taper * 0.9;
		}
	}

	function roundRect(
		ctx: CanvasRenderingContext2D,
		x: number,
		y: number,
		w: number,
		h: number,
		r: number
	) {
		ctx.beginPath();
		ctx.moveTo(x + r, y);
		ctx.arcTo(x + w, y, x + w, y + h, r);
		ctx.arcTo(x + w, y + h, x, y + h, r);
		ctx.arcTo(x, y + h, x, y, r);
		ctx.arcTo(x, y, x + w, y, r);
		ctx.closePath();
	}

	function drawBars(ctx: CanvasRenderingContext2D, w: number, h: number) {
		const gap = Math.max(2, (w / COUNT) * 0.28);
		const bw = (w - gap * (COUNT - 1)) / COUNT;
		const mid = h / 2;
		const r = Math.min(bw / 2, 4);
		ctx.fillStyle = color;
		for (let i = 0; i < COUNT; i++) {
			const v = level[i];
			const bh = Math.max(bw * 0.5, v * v * h * 0.95);
			roundRect(ctx, i * (bw + gap), mid - bh / 2, bw, bh, r);
			ctx.fill();
		}
	}

	function drawRadial(ctx: CanvasRenderingContext2D, w: number, h: number) {
		const cx = w / 2;
		const cy = h / 2;
		const radius = Math.min(w, h) * 0.28;
		ctx.strokeStyle = color;
		ctx.lineCap = 'round';
		ctx.lineWidth = 2.5;
		for (let i = 0; i < COUNT; i++) {
			const v = level[i];
			const len = 6 + v * v * (Math.min(w, h) * 0.22);
			const ang = (i / COUNT) * Math.PI * 2 - Math.PI / 2;
			ctx.beginPath();
			ctx.moveTo(cx + Math.cos(ang) * radius, cy + Math.sin(ang) * radius);
			ctx.lineTo(cx + Math.cos(ang) * (radius + len), cy + Math.sin(ang) * (radius + len));
			ctx.stroke();
		}
	}

	function frame() {
		raf = requestAnimationFrame(frame);
		const cv = canvas;
		if (!cv) return;
		const ctx = cv.getContext('2d');
		if (!ctx) return;
		const { w, h, dpr } = dims(cv);
		ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
		ctx.clearRect(0, 0, w, h);

		if (!realSample(target)) {
			if (fallback) synthSample(target, performance.now());
			else target.fill(0);
		}
		// Ease toward the target for a smooth, non-jittery motion.
		for (let i = 0; i < COUNT; i++) level[i] += (target[i] - level[i]) * 0.35;

		if (variant === 'radial') drawRadial(ctx, w, h);
		else drawBars(ctx, w, h);
	}

	$effect(() => {
		cancelAnimationFrame(raf);
		// Back with an explicit ArrayBuffer so the type matches getByteFrequencyData.
		freq = analyser ? new Uint8Array(new ArrayBuffer(analyser.frequencyBinCount)) : null;
		void fallback; // re-run when fallback toggles
		const cv = canvas;
		if (!cv) return;

		if (!analyser && !fallback) {
			const ctx = cv.getContext('2d');
			if (ctx) {
				const { w, h, dpr } = dims(cv);
				ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
				ctx.clearRect(0, 0, w, h);
			}
			return;
		}
		frame();
		return () => cancelAnimationFrame(raf);
	});

	onDestroy(() => {
		if (typeof cancelAnimationFrame !== 'undefined') cancelAnimationFrame(raf);
	});
</script>

<canvas bind:this={canvas} class={className}></canvas>
