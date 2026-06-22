import { createContext, useCallback, useContext, useRef, useState } from "react";

const ToastCtx = createContext(() => {});
export const useToast = () => useContext(ToastCtx);

// Stacked toasts: each is its own node that auto-expires, so rapid actions
// don't clobber each other (and an Undo isn't lost to the next action).
export function ToastProvider({ children }) {
  const [items, setItems] = useState([]);
  const idRef = useRef(0);

  const remove = useCallback((id) => setItems((xs) => xs.filter((t) => t.id !== id)), []);

  const toast = useCallback((text, undoFn) => {
    const id = ++idRef.current;
    setItems((xs) => [...xs, { id, text, undoFn }]);
    setTimeout(() => remove(id), undoFn ? 6000 : 2400);
  }, [remove]);

  return (
    <ToastCtx.Provider value={toast}>
      {children}
      <div className="toast-stack" id="toast-stack" role="status" aria-live="polite">
        {items.map((t) => (
          <div key={t.id} className="toast show">
            {t.text}
            {t.undoFn && (
              <span className="undo" onClick={() => { t.undoFn(); remove(t.id); }}>Undo</span>
            )}
          </div>
        ))}
      </div>
    </ToastCtx.Provider>
  );
}
