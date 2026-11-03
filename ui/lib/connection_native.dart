import 'package:flutter/material.dart';

import 'api.dart';
import 'native_core.dart';
import 'workspace.dart';

class ConnectionScreen extends StatefulWidget {
  const ConnectionScreen({super.key});
  @override
  State<ConnectionScreen> createState() => _ConnectionScreenState();
}

class _ConnectionScreenState extends State<ConnectionScreen> {
  final _url = TextEditingController(text: 'http://127.0.0.1:8765');
  final _token = TextEditingController();
  ConnectionSettings? _settings;
  NativeCore? _native;
  LensApi? _api;
  String? _error;
  bool _busy = true;
  bool _remote = false;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      _settings = await ConnectionSettings.load();
      _remote = _settings!.data['remote'] == true;
      _url.text = _settings!.data['url'] as String? ?? _url.text;
      _token.text = _settings!.data['token'] as String? ?? '';
      await _connect();
    } catch (error) {
      if (mounted) {
        setState(() {
          _error = '$error';
          _busy = false;
        });
      }
    }
  }

  Future<void> _connect() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final LensApi api;
      if (_remote) {
        final uri = Uri.parse(_url.text.trim());
        if (!['http', 'https'].contains(uri.scheme) || uri.host.isEmpty) {
          throw LensError('Enter an http:// or https:// service URL.');
        }
        api = LensApi(
          _url.text.trim().replaceFirst(RegExp(r'/$'), ''),
          _token.text,
        );
      } else {
        final (native, localApi) = await NativeCore.start(
          _settings!.dataDirectory,
        );
        _native = native;
        api = localApi;
      }
      try {
        await api.call('system.info');
        await _settings!.save({
          'remote': _remote,
          'url': _url.text,
          'token': _token.text,
        });
      } catch (_) {
        api.close();
        await _native?.stop();
        _native = null;
        rethrow;
      }
      if (mounted) {
        setState(() {
          _api = api;
          _busy = false;
        });
      }
    } catch (error) {
      if (mounted) {
        setState(() {
          _error = '$error';
          _busy = false;
        });
      }
    }
  }

  Future<void> _disconnect() async {
    _api?.close();
    try {
      await _native?.stop();
    } catch (error) {
      _error = '$error';
    }
    _native = null;
    setState(() => _api = null);
  }

  @override
  void dispose() {
    _api?.close();
    _url.dispose();
    _token.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    if (_api != null) return Workspace(api: _api!, onDisconnect: _disconnect);
    return Scaffold(
      appBar: AppBar(title: const Text('eCaptureLens')),
      body: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 520),
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(24),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Text(
                  'Your traffic, in focus.',
                  style: Theme.of(context).textTheme.headlineMedium,
                ),
                const SizedBox(height: 12),
                const Text(
                  'Inspect Android eCapture requests locally, or connect to an eCaptureLens service.',
                ),
                const SizedBox(height: 28),
                SegmentedButton<bool>(
                  segments: const [
                    ButtonSegment(
                      value: false,
                      icon: Icon(Icons.computer_outlined),
                      label: Text('This device'),
                    ),
                    ButtonSegment(
                      value: true,
                      icon: Icon(Icons.lan_outlined),
                      label: Text('Remote service'),
                    ),
                  ],
                  selected: {_remote},
                  onSelectionChanged: _busy
                      ? null
                      : (value) => setState(() => _remote = value.first),
                ),
                const SizedBox(height: 24),
                if (_remote) ...[
                  TextField(
                    controller: _url,
                    decoration: const InputDecoration(labelText: 'Service URL'),
                  ),
                  const SizedBox(height: 16),
                  TextField(
                    controller: _token,
                    obscureText: true,
                    decoration: const InputDecoration(labelText: 'API token'),
                  ),
                ] else
                  const Text(
                    'The embedded Go core stores the latest 1,000 requests privately on this device. Android capture requires root and an installed eCapture binary.',
                  ),
                if (_error != null)
                  Padding(
                    padding: const EdgeInsets.symmetric(vertical: 20),
                    child: SelectableText(
                      _error!,
                      style: TextStyle(
                        color: Theme.of(context).colorScheme.error,
                      ),
                    ),
                  ),
                const SizedBox(height: 24),
                FilledButton.icon(
                  onPressed: _busy || _settings == null ? null : _connect,
                  icon: _busy
                      ? const SizedBox(
                          width: 18,
                          height: 18,
                          child: CircularProgressIndicator(strokeWidth: 2),
                        )
                      : const Icon(Icons.arrow_forward),
                  label: Text(_busy ? 'Connecting…' : 'Open workspace'),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
