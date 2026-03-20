import { useCallback, useEffect, useRef, useState } from 'react';
import {
  enhancePrompt,
  type EnhanceAnswer,
  type EnhanceQuestion,
  type ReasoningEffort,
} from '../lib/api';

type PromptEnhanceModalProps = {
  isOpen: boolean;
  onClose: () => void;
  prompt: string;
  modelId: string;
  reasoningEffort?: ReasoningEffort;
  onUsePrompt: (enhancedPrompt: string) => void;
};

type ModalState = 'loading_questions' | 'questions' | 'loading_enhance' | 'result' | 'error';

export default function PromptEnhanceModal({
  isOpen,
  onClose,
  prompt,
  modelId,
  reasoningEffort,
  onUsePrompt,
}: PromptEnhanceModalProps) {
  const [modalState, setModalState] = useState<ModalState>('loading_questions');
  const [questions, setQuestions] = useState<EnhanceQuestion[]>([]);
  const [answers, setAnswers] = useState<Map<string, string[]>>(new Map());
  const [enhancedPrompt, setEnhancedPrompt] = useState('');
  const [iteration, setIteration] = useState(0);
  const [qaHistory, setQaHistory] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const abortControllerRef = useRef<AbortController | null>(null);

  // Cancel any in-flight requests on unmount or close.
  const cancelRequests = useCallback(() => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
    }
  }, []);

  // Fetch initial questions on open.
  useEffect(() => {
    if (!isOpen) return;

    const controller = new AbortController();
    abortControllerRef.current = controller;

    setModalState('loading_questions');
    setQuestions([]);
    setAnswers(new Map());
    setEnhancedPrompt('');
    setIteration(0);
    setQaHistory('');
    setError(null);

    enhancePrompt(
      { prompt, modelId, reasoningEffort },
      { signal: controller.signal },
    )
      .then((response) => {
        if (controller.signal.aborted) return;
        if (response.questions && response.questions.length > 0) {
          setQuestions(response.questions);
          setModalState('questions');
        } else {
          setError('No questions returned. Try a more detailed prompt.');
          setModalState('error');
        }
      })
      .catch((err) => {
        if (controller.signal.aborted) return;
        setError((err as Error).message || 'Failed to analyze prompt');
        setModalState('error');
      });

    return () => {
      controller.abort();
    };
  }, [isOpen, prompt, modelId, reasoningEffort]);

  // Escape key closes modal.
  useEffect(() => {
    if (!isOpen) return;
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        cancelRequests();
        onClose();
      }
    }
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, onClose, cancelRequests]);

  function handleBackdropClick(e: React.MouseEvent<HTMLDivElement>) {
    if (e.target === e.currentTarget) {
      cancelRequests();
      onClose();
    }
  }

  function handleCloseClick() {
    cancelRequests();
    onClose();
  }

  function handleOptionSelect(questionId: string, optionId: string, questionType: string) {
    setAnswers((prev) => {
      const next = new Map(prev);
      if (questionType === 'single_select' || questionType === 'yes_no') {
        next.set(questionId, [optionId]);
      } else {
        // multi_select: toggle
        const current = next.get(questionId) ?? [];
        if (current.includes(optionId)) {
          const filtered = current.filter((id) => id !== optionId);
          if (filtered.length === 0) {
            next.delete(questionId);
          } else {
            next.set(questionId, filtered);
          }
        } else {
          next.set(questionId, [...current, optionId]);
        }
      }
      return next;
    });
  }

  function serializeQAHistory(): string {
    const lines: string[] = [];
    for (const q of questions) {
      const selected = answers.get(q.id) ?? [];
      const selectedLabels = q.options
        .filter((opt) => selected.includes(opt.id))
        .map((opt) => opt.label);
      if (selectedLabels.length > 0) {
        lines.push(`Q: ${q.text}`);
        lines.push(`A: ${selectedLabels.join(', ')}`);
      }
    }
    return lines.join('\n');
  }

  async function handleEnhance() {
    setModalState('loading_enhance');
    const controller = new AbortController();
    abortControllerRef.current = controller;

    const answersPayload: EnhanceAnswer[] = questions
      .filter((q) => answers.has(q.id))
      .map((q) => ({
        questionId: q.id,
        questionText: q.text,
        selectedOptions: answers.get(q.id) ?? [],
      }));

    try {
      const response = await enhancePrompt(
        {
          prompt,
          modelId,
          reasoningEffort,
          answers: answersPayload,
          previousEnhancedPrompt: enhancedPrompt || undefined,
          previousQuestionsAndAnswers: qaHistory || undefined,
          iteration,
        },
        { signal: controller.signal },
      );

      if (controller.signal.aborted) return;

      if (response.enhancedPrompt) {
        setEnhancedPrompt(response.enhancedPrompt);
        setModalState('result');
      } else {
        setError('No enhanced prompt returned.');
        setModalState('error');
      }
    } catch (err) {
      if (controller.signal.aborted) return;
      setError((err as Error).message || 'Failed to enhance prompt');
      setModalState('error');
    }
  }

  async function handleGoDeeper() {
    if (iteration >= 3) return;

    const currentQA = serializeQAHistory();
    const combinedHistory = qaHistory ? `${qaHistory}\n\n${currentQA}` : currentQA;
    setQaHistory(combinedHistory);

    const nextIteration = iteration + 1;
    setIteration(nextIteration);
    setModalState('loading_questions');
    setQuestions([]);
    setAnswers(new Map());

    const controller = new AbortController();
    abortControllerRef.current = controller;

    try {
      const response = await enhancePrompt(
        {
          prompt,
          modelId,
          reasoningEffort,
          previousEnhancedPrompt: enhancedPrompt,
          previousQuestionsAndAnswers: combinedHistory,
          iteration: nextIteration,
        },
        { signal: controller.signal },
      );

      if (controller.signal.aborted) return;

      if (response.questions && response.questions.length > 0) {
        setQuestions(response.questions);
        setModalState('questions');
      } else {
        setError('No follow-up questions returned.');
        setModalState('error');
      }
    } catch (err) {
      if (controller.signal.aborted) return;
      setError((err as Error).message || 'Failed to generate deeper questions');
      setModalState('error');
    }
  }

  function handleUsePrompt() {
    onUsePrompt(enhancedPrompt);
  }

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(enhancedPrompt);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Fallback: do nothing
    }
  }

  function handleRetry() {
    setError(null);
    setModalState('loading_questions');

    const controller = new AbortController();
    abortControllerRef.current = controller;

    enhancePrompt(
      {
        prompt,
        modelId,
        reasoningEffort,
        previousEnhancedPrompt: enhancedPrompt || undefined,
        previousQuestionsAndAnswers: qaHistory || undefined,
        iteration,
      },
      { signal: controller.signal },
    )
      .then((response) => {
        if (controller.signal.aborted) return;
        if (response.questions && response.questions.length > 0) {
          setQuestions(response.questions);
          setModalState('questions');
        } else if (response.enhancedPrompt) {
          setEnhancedPrompt(response.enhancedPrompt);
          setModalState('result');
        } else {
          setError('Unexpected response. Please try again.');
          setModalState('error');
        }
      })
      .catch((err) => {
        if (controller.signal.aborted) return;
        setError((err as Error).message || 'Failed to analyze prompt');
        setModalState('error');
      });
  }

  if (!isOpen) return null;

  const hasAllAnswers = questions.length > 0 && questions.every((q) => (answers.get(q.id) ?? []).length > 0);

  return (
    <div className="enhance-modal-backdrop" onClick={handleBackdropClick}>
      <div className="enhance-modal">
        <div className="enhance-modal-header">
          <h3>Enhance Prompt</h3>
          <button className="enhance-modal-close" onClick={handleCloseClick} aria-label="Close">
            &times;
          </button>
        </div>

        <div className="enhance-modal-body">
          {(modalState === 'loading_questions' || modalState === 'loading_enhance') && (
            <div className="enhance-loading">
              <div className="enhance-spinner" />
              <p>
                {modalState === 'loading_questions'
                  ? 'Analyzing your prompt...'
                  : 'Enhancing your prompt...'}
              </p>
            </div>
          )}

          {modalState === 'questions' && (
            <div className="enhance-questions">
              {questions.map((q) => (
                <div key={q.id} className="enhance-question-card">
                  <p className="enhance-question-text">{q.text}</p>
                  <div className="enhance-options">
                    {q.options.map((opt) => {
                      const selected = (answers.get(q.id) ?? []).includes(opt.id);
                      return (
                        <button
                          key={opt.id}
                          type="button"
                          className={`enhance-option-chip ${selected ? 'selected' : ''}`}
                          onClick={() => handleOptionSelect(q.id, opt.id, q.type)}
                        >
                          {opt.label}
                        </button>
                      );
                    })}
                  </div>
                </div>
              ))}
            </div>
          )}

          {modalState === 'result' && (
            <div className="enhance-result">
              <div className="enhance-result-text">{enhancedPrompt}</div>
              <div className="enhance-result-actions">
                <button className="btn-enhance-use" onClick={handleUsePrompt}>
                  Use this prompt
                </button>
                <button className="btn-enhance-copy" onClick={handleCopy}>
                  {copied ? 'Copied!' : 'Copy'}
                </button>
                {iteration < 3 && (
                  <button className="btn-enhance-deeper" onClick={handleGoDeeper}>
                    Go Deeper
                  </button>
                )}
              </div>
            </div>
          )}

          {modalState === 'error' && (
            <div className="enhance-error">
              <p>{error}</p>
              <button className="btn-enhance-retry" onClick={handleRetry}>
                Retry
              </button>
            </div>
          )}
        </div>

        {modalState === 'questions' && (
          <div className="enhance-modal-footer">
            <button
              className="btn-enhance-submit"
              disabled={!hasAllAnswers}
              onClick={handleEnhance}
            >
              Enhance
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
