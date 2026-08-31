/**
 * One slide, large.
 *
 * The whole component is a scrim and a picture. There is no frame around the
 * image, no caption and no controls, because there is nothing here to operate:
 * you opened it to look at something, and every pixel a title bar took would be
 * a pixel off the thing you opened.
 *
 * Clicking anywhere closes it. The scrim *is* the button — which is why the
 * image sits inside it rather than beside it, and why there is no close box in
 * a corner competing to be the way out.
 *
 * `object-contain` and a cap on both axes: the slide keeps its shape, and a tall
 * window and a wide one both show the whole of it.
 */
export function SlideViewer({
  src,
  alt,
  onClose,
}: {
  src: string
  alt: string
  onClose: () => void
}) {
  return (
    <div
      onClick={onClose}
      className="fixed inset-0 z-50 flex items-center justify-center p-6"
      style={{ backgroundColor: 'rgb(0 0 0 / 0.55)' }}
    >
      <img
        src={src}
        alt={alt}
        className="max-h-[88vh] max-w-[92vw] rounded-[10px] object-contain"
        style={{ boxShadow: '0 24px 64px -12px rgb(0 0 0 / 0.5)' }}
      />
    </div>
  )
}
