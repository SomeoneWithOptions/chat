type ToggleProps = {
  checked: boolean;
  onChange: (checked: boolean) => void;
  label: string;
  disabled?: boolean;
};

export default function Toggle({ checked, onChange, label, disabled }: ToggleProps) {
  return (
    <label
      className="toggle"
      style={{ opacity: disabled ? 0.4 : 1, pointerEvents: disabled ? 'none' : 'auto' }}
    >
      <div
        className={`toggle-track ${checked ? 'active' : ''}`}
        onClick={(e) => {
          e.preventDefault();
          if (!disabled) onChange(!checked);
        }}
        role="switch"
        aria-checked={checked}
        tabIndex={0}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            if (!disabled) onChange(!checked);
          }
        }}
      >
        <div className="toggle-thumb" />
      </div>
      <span className="toggle-label">{label}</span>
    </label>
  );
}
