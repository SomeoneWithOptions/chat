import { type Conversation } from '../lib/api';

type SidebarProps = {
  conversations: Conversation[];
  activeConversationId: string | null;
  onSelectConversation: (id: string) => void;
  onNewChat: () => void;
  onDeleteConversation: (id: string) => void;
  onDeleteAllConversations: () => void;
  onLogout: () => void;
  userEmail: string;
  isStreaming: boolean;
  deletingConversationId: string | null;
  isDeletingAll: boolean;
  loadingConversations: boolean;
  conversationAPISupported: boolean;
  collapsed: boolean;
  onToggleCollapsed: () => void;
};

import { useState } from 'react';

export default function Sidebar({
  conversations,
  activeConversationId,
  onSelectConversation,
  onNewChat,
  onDeleteConversation,
  onDeleteAllConversations,
  onLogout,
  userEmail,
  isStreaming,
  deletingConversationId,
  isDeletingAll,
  loadingConversations,
  conversationAPISupported,
  collapsed,
  onToggleCollapsed,
}: SidebarProps) {
  const [conversationsOpen, setConversationsOpen] = useState(true);

  const canInteract = !isStreaming && deletingConversationId === null && !isDeletingAll;

  return (
    <>
      {/* Mobile overlay */}
      {!collapsed && (
        <div
          className="mobile-overlay"
          onClick={onToggleCollapsed}
          style={{ display: 'none' }}
        />
      )}

      <aside className={`sidebar ${collapsed ? 'collapsed' : ''}`}>
        <div className="sidebar-header">
          <span className="sidebar-brand">
            Saneto <em>Chat</em>
          </span>
          <button
            className="btn-icon"
            onClick={onToggleCollapsed}
            aria-label="Close sidebar"
          >
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M15 18l-6-6 6-6" />
            </svg>
          </button>
        </div>

        {conversationAPISupported && (
          <button
            className="btn-new-chat"
            onClick={onNewChat}
            disabled={!canInteract}
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <line x1="12" y1="5" x2="12" y2="19" />
              <line x1="5" y1="12" x2="19" y2="12" />
            </svg>
            New conversation
          </button>
        )}

        {conversationAPISupported && (
          <div className="sidebar-section conversations-section">
            <button
              className="section-toggle"
              onClick={() => setConversationsOpen(!conversationsOpen)}
            >
              <span>Conversations</span>
              <svg
                className={`section-toggle-icon ${conversationsOpen ? 'open' : ''}`}
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
              >
                <polyline points="6 9 12 15 18 9" />
              </svg>
            </button>

            <div className={`section-content ${conversationsOpen ? 'open' : 'collapsed'}`}
                 style={conversationsOpen ? { maxHeight: '9999px' } : undefined}>
              {loadingConversations && (
                <div style={{ padding: '12px 20px', fontSize: '12px', color: 'var(--text-tertiary)' }}>
                  Loading...
                </div>
              )}

              {!loadingConversations && conversations.length === 0 && (
                <div style={{ padding: '12px 20px', fontSize: '12px', color: 'var(--text-tertiary)' }}>
                  No conversations yet
                </div>
              )}

              {!loadingConversations && conversations.length > 0 && (
                <div className="conversation-list">
                  {conversations.map((conversation) => (
                    <div
                      key={conversation.id}
                      className={`conversation-item ${activeConversationId === conversation.id ? 'active' : ''}`}
                      onClick={() => canInteract && onSelectConversation(conversation.id)}
                      style={{ position: 'relative' }}
                    >
                      <span className="conversation-title">{conversation.title}</span>
                      <button
                        className="conversation-delete-btn"
                        onClick={(e) => {
                          e.stopPropagation();
                          onDeleteConversation(conversation.id);
                        }}
                        disabled={!canInteract}
                        aria-label="Delete conversation"
                      >
                        {deletingConversationId === conversation.id ? (
                          <span style={{ fontSize: '10px' }}>...</span>
                        ) : (
                          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                            <polyline points="3 6 5 6 21 6" />
                            <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
                          </svg>
                        )}
                      </button>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        )}

        <div className="sidebar-footer">
          <div className="sidebar-footer-actions">
            <span className="sidebar-user" title={userEmail}>{userEmail}</span>
            <div style={{ display: 'flex', gap: '4px' }}>
              {conversationAPISupported && conversations.length > 0 && (
                <button
                  className="btn-icon danger"
                  onClick={onDeleteAllConversations}
                  disabled={!canInteract}
                  title="Delete all conversations"
                  aria-label="Delete all conversations"
                >
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                    <polyline points="3 6 5 6 21 6" />
                    <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
                  </svg>
                </button>
              )}
              <button
                className="btn-icon"
                onClick={onLogout}
                title="Sign out"
                aria-label="Sign out"
              >
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
                  <polyline points="16 17 21 12 16 7" />
                  <line x1="21" y1="12" x2="9" y2="12" />
                </svg>
              </button>
            </div>
          </div>
        </div>
      </aside>
    </>
  );
}
