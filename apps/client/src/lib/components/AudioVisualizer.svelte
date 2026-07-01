<script lang="ts">
	import { onDestroy } from 'svelte';

	// The page owns the AudioContext + audio graph and passes a live AnalyserNode.
	// When `analyser` is null the canvas idles (cleared). Works for both the mic
	// (régie) and the listener stream.
	let {
		analyser = null,
		variant = 'bars',
		color = 'rgba(250,250,250,0.92)',
		class: className = ''
	}: {
		analyser?: AnalyserNode | null;
		variant?: 'bars' | 'radial';
		color?: string;
		class?: string;
	} = $props();

	let canvas: HTMLCanvasElement | undefined = $state();
	let raf = 0;

	function resize(cv: HTMLCanvasElement) {
		const dpr = Math.min(window.devicePixelRatio || 1, 2);
		const w = cv.clientWidth;
		const h = cv.clientHeight;
		if (cv.width !== Math.round(w * dpr) || cv.height !== Math.round(h * dpr)) {
			cv.width = Math.round(w * dpr);
			cv.height = Math.round(h * dpr);
		}
		return { w, h, dpr };
	}

	function drawBars(ctx: CanvasRenderingContext2D, data: Uint8Array, w: number, h: number, dpr: number) {
		ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
		ctx.clearRect(0, 0, w, h);
		const count = Math.min(64, data.length);
		const gap = 3;
		const bw = (w - gap * (count - 1)) / count;
		const mid = h / 2;
		ctx.fillStyle = color;
		for (let i = 0; i < count; i++) {
			// emphasise lower-mid frequencies (voice/music energy)
			const v = data[Math.floor((i / count) * data.length * 0.7)] / 255;
			const bh = Math.max(bw * 0.6, v * v * h);
			const x = i * (bw + gap);
			const r = Math.min(bw / 2, 4);
			roundRect(ctx, x, mid - bh / 2, bw, bh, r);
			ctx.fill();
		}
	}

	function drawRadial(ctx: CanvasRenderingContext2D, data: Uint8Array, w: number, h: number, dpr: number) {
		ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
		ctx.clearRect(0, 0, w, h);
		const cx = w / 2;
		const cy = h / 2;
		const count = 72;
		const radius = Math.min(w, h) * 0.28;
		ctx.strokeStyle = color;
		ctx.lineCap = 'round';
		for (let i = 0; i < count; i++) {
			const v = data[Math.floor((i / count) * data.length * 0.7)] / 255;
			const len = 6 + v * v * (Math.min(w, h) * 0.22);
			const a = (i / count) * Math.PI * 2 - Math.PI / 2;
			const x1 = cx + Math.cos(a) * radius;
			const y1 = cy + Math.sin(a) * radius;
			const x2 = cx + Math.cos(a) * (radius + len);
			const y2 = cy + Math.sin(a) * (radius + len);
			ctx.lineWidth = 2.5;
			ctx.beginPath();
			ctx.moveTo(x1, y1);
			ctx.lineTo(x2, y2);
			ctx.stroke();
		}
	}

	function roundRect(ctx: CanvasRenderingContext2D, x: number, y: number, w: number, h: number, r: number) {
		ctx.beginPath();
		ctx.moveTo(x + r, y);
		ctx.arcTo(x + w, y, x + w, y + h, r);
		ctx.arcTo(x + w, y + h, x, y + h, r);
		ctx.arcTo(x, y + h, x, y, r);
		ctx.arcTo(x, y, x + w, y, r);
		ctx.closePath();
	}

	$effect(() => {
		cancelAnimationFrame(raf);
		const cv = canvas;
		if (!cv) return;
		const ctx = cv.getContext('2d');
		if (!ctx) return;

		if (!analyser) {
			const { w, h, dpr } = resize(cv);
			ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
			ctx.clearRect(0, 0, w, h);
			return;
		}

		const data = new Uint8Array(analyser.frequencyBinCount);
		const loop = () => {
			raf = requestAnimationFrame(loop);
			const { w, h, dpr } = resize(cv);
			analyser!.getByteFrequencyData(data);
			if (variant === 'radial') drawRadial(ctx, data, w, h, dpr);
			else drawBars(ctx, data, w, h, dpr);
		};
		loop();

		return () => cancelAnimationFrame(raf);
	});

	onDestroy(() => cancelAnimationFrame(raf));
</script>

<canvas bind:this={canvas} class={className}></canvas>
