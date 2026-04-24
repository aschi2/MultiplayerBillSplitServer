<script lang="ts">
  import { browser } from '$app/environment';
  import { createEventDispatcher, onDestroy, tick } from 'svelte';
  import 'cropperjs/dist/cropper.css';
  import type Cropper from 'cropperjs';

  export let open = false;
  export let file: File | null = null;
  export let busy = false;

  const dispatch = createEventDispatcher<{
    cancel: void;
    confirm: { file: File; cropped: boolean };
  }>();

  let imageEl: HTMLImageElement | null = null;
  let imageUrl = '';
  let cropper: Cropper | null = null;
  let cropperReady = false;
  let cropError: string | null = null;
  let cropWorking = false;
  let loadedFileKey = '';
  let CropperCtor: typeof import('cropperjs').default | null = null;

  const receiptOutputType = (source: File) => {
    if (source.type === 'image/png') return 'image/png';
    if (source.type === 'image/webp') return 'image/webp';
    return 'image/jpeg';
  };

  const receiptOutputExtension = (outputType: string) => {
    if (outputType === 'image/png') return 'png';
    if (outputType === 'image/webp') return 'webp';
    return 'jpg';
  };

  const cleanupCropper = () => {
    cropper?.destroy();
    cropper = null;
    cropperReady = false;
  };

  const cleanupPreview = () => {
    cleanupCropper();
    if (imageUrl) {
      URL.revokeObjectURL(imageUrl);
      imageUrl = '';
    }
    loadedFileKey = '';
    cropError = null;
    rotationAngle = 0;
  };

  const ensureCropperCtor = async () => {
    if (!browser) return null;
    if (!CropperCtor) {
      const module = await import('cropperjs');
      CropperCtor = module.default;
    }
    return CropperCtor;
  };

  const initializeCropper = async () => {
    if (!browser || !open || !file || !imageEl || !imageUrl) return;
    const CropperClass = await ensureCropperCtor();
    if (!CropperClass || !imageEl) return;
    cleanupCropper();
    cropper = new CropperClass(imageEl, {
      viewMode: 1,
      dragMode: 'move',
      background: false,
      responsive: true,
      restore: false,
      checkOrientation: true,
      autoCropArea: 0.92,
      movable: true,
      zoomable: true,
      rotatable: true,
      scalable: false,
      guides: true,
      center: true,
      highlight: false,
      toggleDragModeOnDblclick: false,
      ready: () => {
        cropperReady = true;
      }
    });
  };

  const loadFileIntoCropper = async (nextFile: File) => {
    cleanupPreview();
    cropError = null;
    imageUrl = URL.createObjectURL(nextFile);
    await tick();
    await initializeCropper();
  };

  $: if (open && file) {
    const nextKey = `${file.name}:${file.size}:${file.lastModified}`;
    if (nextKey !== loadedFileKey) {
      loadedFileKey = nextKey;
      void loadFileIntoCropper(file);
    }
  }

  $: if (!open && (imageUrl || cropper)) {
    cleanupPreview();
  }

  onDestroy(() => {
    cleanupPreview();
  });

  const cancel = () => {
    if (busy || cropWorking) return;
    cleanupPreview();
    dispatch('cancel');
  };

  let rotationAngle = 0;

  const setRotation = (degrees: number) => {
    rotationAngle = degrees;
    cropper?.rotateTo(degrees);
  };

  const reset = () => {
    cropper?.reset();
    rotationAngle = 0;
  };

  const confirmCurrentCrop = async (useOriginal = false) => {
    if (!file || busy || cropWorking) return;
    cropError = null;
    cropWorking = true;
    try {
      let nextFile = file;
      if (!useOriginal) {
        if (!cropper) {
          throw new Error('Crop tool is not ready yet.');
        }
        const canvas = cropper.getCroppedCanvas({
          imageSmoothingEnabled: true,
          imageSmoothingQuality: 'high',
          fillColor: '#ffffff'
        });
        if (!canvas) {
          throw new Error('Failed to crop image.');
        }
        const outputType = receiptOutputType(file);
        const outputQuality = outputType === 'image/png' ? undefined : 0.98;
        const blob = await new Promise<Blob | null>((resolve) =>
          canvas.toBlob(resolve, outputType, outputQuality)
        );
        if (!blob) {
          throw new Error('Failed to export cropped image.');
        }
        const baseName = file.name.replace(/\.\w+$/, '') || 'receipt';
        nextFile = new File([blob], `${baseName}-cropped.${receiptOutputExtension(outputType)}`, {
          type: outputType
        });
      }
      cleanupPreview();
      dispatch('confirm', { file: nextFile, cropped: !useOriginal });
    } catch (err) {
      cropError = err instanceof Error ? err.message : 'Failed to prepare receipt image.';
    } finally {
      cropWorking = false;
    }
  };
</script>

{#if open && file}
  <div class="modal-scrim">
    <div class="glass-card bottom-sheet ui-bottom-sheet max-h-[88vh] space-y-4">
      <div class="flex items-start justify-between gap-3">
        <div>
          <h3 class="text-lg font-semibold modal-title">Crop receipt</h3>
          <p class="text-sm text-surface-200 modal-subtitle">
            Adjust the frame and rotate if needed before parsing.
          </p>
        </div>
        <button class="action-btn action-btn-surface action-btn-compact shrink-0" type="button" on:click={cancel} disabled={busy || cropWorking}>
          Cancel
        </button>
      </div>

      <div class="rounded-2xl border border-white/10 bg-black/40 p-2 ui-panel">
        <div class="overflow-hidden rounded-xl bg-black/50 min-h-[42vh] max-h-[56vh] flex items-center justify-center">
          {#if imageUrl}
            <img bind:this={imageEl} src={imageUrl} alt="Receipt crop preview" class="block max-h-[56vh] w-full object-contain" />
          {/if}
        </div>
      </div>

      {#if cropError}
        <div class="rounded-xl border border-error-500/40 bg-error-500/15 px-3 py-2 text-sm text-error-100 ui-panel">
          {cropError}
        </div>
      {/if}

      {#if cropperReady}
        <div class="flex items-center gap-3">
          <input
            type="range"
            min="-180"
            max="180"
            step="0.5"
            value={rotationAngle}
            on:input={(e) => setRotation(Number(e.currentTarget.value))}
            disabled={busy || cropWorking}
            class="rotation-slider flex-1"
          />
          <span class="text-xs text-surface-300 tabular-nums w-12 text-right shrink-0">{rotationAngle.toFixed(1)}&deg;</span>
          <button class="action-btn action-btn-surface action-btn-compact" type="button" on:click={reset} disabled={busy || cropWorking}>
            Reset
          </button>
        </div>
      {/if}

      <div class="flex flex-col-reverse gap-2 sm:flex-row sm:justify-between">
        <button class="action-btn action-btn-surface" type="button" on:click={() => confirmCurrentCrop(true)} disabled={busy || cropWorking}>
          Use original photo
        </button>
        <button class="btn btn-primary" type="button" on:click={() => confirmCurrentCrop(false)} disabled={!cropperReady || busy || cropWorking}>
          {cropWorking || busy ? 'Preparing…' : 'Parse cropped receipt'}
        </button>
      </div>
    </div>
  </div>
{/if}

<style>
  .rotation-slider {
    -webkit-appearance: none;
    appearance: none;
    height: 4px;
    border-radius: 2px;
    background: rgba(255, 255, 255, 0.12);
    outline: none;
  }
  .rotation-slider::-webkit-slider-thumb {
    -webkit-appearance: none;
    appearance: none;
    width: 20px;
    height: 20px;
    border-radius: 50%;
    background: rgb(45, 212, 191);
    cursor: pointer;
    border: 2px solid rgba(255, 255, 255, 0.2);
  }
  .rotation-slider::-moz-range-thumb {
    width: 20px;
    height: 20px;
    border-radius: 50%;
    background: rgb(45, 212, 191);
    cursor: pointer;
    border: 2px solid rgba(255, 255, 255, 0.2);
  }
  .rotation-slider:disabled {
    opacity: 0.4;
  }
</style>
