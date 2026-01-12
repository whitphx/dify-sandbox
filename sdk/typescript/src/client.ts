import {
  SandboxResponse,
  RunCodeResponse,
  RunOptions,
  UploadFileResponse,
  UploadFileOptions,
} from './types';

export class SandboxClient {
  private baseUrl: string;
  private apiKey: string;

  constructor(baseUrl: string, apiKey: string) {
    this.baseUrl = baseUrl.replace(/\/$/, '');
    this.apiKey = apiKey;
  }

  private async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const url = `${this.baseUrl}${path}`;
    const headers: Record<string, string> = {
      'X-API-KEY': this.apiKey,
      ...((options.headers as Record<string, string>) || {}),
    };

    const response = await fetch(url, {
      ...options,
      headers,
    });

    if (!response.ok) {
      let errorMsg = `Request failed with status ${response.status}: ${response.statusText}`;
      try {
        const json = (await response.json()) as SandboxResponse<T>;
        if (json.code !== 0) {
          const msg = json.message || 'Unknown error'; // Types.ts says it has message.
          errorMsg = `Sandbox API error (${json.code}): ${msg}`;
        } else if (json.message) {
          errorMsg = `Request failed with status ${response.status}: ${json.message}`;
        }
      } catch {
        // If response is not JSON, use the default error message
      }
      throw new Error(errorMsg);
    }

    const json = (await response.json()) as SandboxResponse<T>;
    if (json.code !== 0) {
      const msg = json.message || 'Unknown error';
      throw new Error(`Sandbox API error (${json.code}): ${msg}`);
    }

    return json.data;
  }

  /**
   * Upload a file to the sandbox storage.
   * @param file Blob or File to upload
   * @param filename Optional filename for the file
   * @param options Optional upload options (e.g., TTL)
   */
  async uploadFile(
    file: File | Blob,
    filename?: string,
    options?: UploadFileOptions
  ): Promise<{ file_id: string }> {
    const formData = new FormData();
    formData.append('file', file, filename); // 'file' is the field name expected by Gin

    // Add TTL if specified
    if (options?.ttl !== undefined && options.ttl > 0) {
      formData.append('ttl', options.ttl.toString());
    }

    const result = await this.request<UploadFileResponse>('/v1/sandbox/files', {
      method: 'POST',
      body: formData,
      // Header Content-Type is set automatically by fetch when body is FormData
    });
    return result;
  }

  /**
   * Download a file from the sandbox storage.
   * Returns a Blob. In Node.js environment this is a Blob.
   */
  async downloadFile(fileId: string): Promise<Blob> {
    const url = `${this.baseUrl}/v1/sandbox/files/${fileId}`;
    const response = await fetch(url, {
      method: 'GET',
      headers: {
        'X-API-KEY': this.apiKey,
      },
    });

    if (!response.ok) {
      throw new Error(`Download failed with status ${response.status}`);
    }
    return await response.blob();
  }

  // Helper to get text from download
  async downloadFileText(fileId: string): Promise<string> {
    const blob = await this.downloadFile(fileId);
    return await blob.text();
  }

  async runPython(code: string, options: RunOptions = {}): Promise<RunCodeResponse> {
    const body = {
      language: 'python3',
      code,
      preload: options.preload,
      enable_network: options.enable_network,
      files: options.input_files, // Map<filename, file_id>
      fetch_files: options.fetch_files,
    };

    return await this.request<RunCodeResponse>('/v1/sandbox/run', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
  }

  async runNodeJs(code: string, options: RunOptions = {}): Promise<RunCodeResponse> {
    const body = {
      language: 'nodejs',
      code,
      preload: options.preload,
      enable_network: options.enable_network,
      files: options.input_files,
      fetch_files: options.fetch_files,
    };

    return await this.request<RunCodeResponse>('/v1/sandbox/run', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
  }
}
