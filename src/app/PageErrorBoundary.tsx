import React from 'react';
import { ErrorState } from '../components/States';

interface Props {
  /** Changing this value (the current path) clears a caught error. */
  resetKey: string;
  children: React.ReactNode;
}

interface State {
  error: Error | null;
}

/**
 * Keeps a page that throws while rendering from blanking the whole console:
 * the shell stays up and the page area shows the error with a way to retry.
 */
export class PageErrorBoundary extends React.Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidUpdate(prev: Props) {
    if (prev.resetKey !== this.props.resetKey && this.state.error) {
      this.setState({ error: null });
    }
  }

  render() {
    if (this.state.error) {
      return (
        <ErrorState
          title="This page couldn't be shown"
          message={this.state.error.message}
          onRetry={() => this.setState({ error: null })}
        />
      );
    }
    return this.props.children;
  }
}
