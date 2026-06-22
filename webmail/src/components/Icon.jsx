// Inline SVG icon. `body` is raw inner-SVG markup (trusted, app-defined).
export default function Icon({ body, className = "ic", style }) {
  return (
    <svg viewBox="0 0 24 24" className={className} style={style} dangerouslySetInnerHTML={{ __html: body }} />
  );
}
