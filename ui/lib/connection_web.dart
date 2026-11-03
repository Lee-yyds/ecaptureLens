import 'package:flutter/material.dart';
import 'package:web/web.dart' as web;

import 'api.dart';
import 'workspace.dart';

class ConnectionSettings {
  ConnectionSettings(this.url, this.token);
  final String url;
  String token;

  static ConnectionSettings load() {
    var token = web.window.sessionStorage.getItem('ecapturelens.token') ?? '';
    final fragment = web.window.location.hash;
    if (fragment.isNotEmpty) {
      web.window.history.replaceState(
        web.window.history.state,
        '',
        '${web.window.location.pathname}${web.window.location.search}',
      );
      final values = Uri.splitQueryString(fragment.substring(1));
      if (values.containsKey('token')) token = values['token']!;
    }
    final settings = ConnectionSettings(web.window.location.origin, token);
    settings.save(token);
    return settings;
  }

  void save(String value) {
    if (value.isEmpty) {
      web.window.sessionStorage.removeItem('ecapturelens.token');
    } else {
      web.window.sessionStorage.setItem('ecapturelens.token', value);
    }
    token = value;
  }
}

class ConnectionScreen extends StatefulWidget {
  const ConnectionScreen({super.key});

  @override
  State<ConnectionScreen> createState() => _ConnectionScreenState();
}

class _ConnectionScreenState extends State<ConnectionScreen> {
  final _token = TextEditingController();
  ConnectionSettings? _settings;
  LensApi? _api;
  String? _error;
  bool _busy = false;

  @override
  void initState() {
    super.initState();
    try {
      _settings = ConnectionSettings.load();
      _token.text = _settings!.token;
      if (_token.text.isNotEmpty) _connect();
    } catch (error) {
      _error = '$error';
    }
  }

  Future<void> _connect() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    final api = LensApi(_settings!.url, _token.text);
    try {
      await api.call('system.info');
      _settings!.save(_token.text);
      if (mounted) setState(() => _api = api);
    } catch (error) {
      api.close();
      if (mounted) setState(() => _error = '$error');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  void _disconnect() {
    _api!.close();
    _settings!.save('');
    _token.clear();
    setState(() => _api = null);
  }

  @override
  void dispose() {
    _api?.close();
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
                  'Open the launch URL printed by ecapturelens web, or enter its API token below. Capture commands and history run in the Go service.',
                ),
                const SizedBox(height: 24),
                SelectableText('Service: ${_settings?.url ?? Uri.base.origin}'),
                const SizedBox(height: 16),
                TextField(
                  controller: _token,
                  obscureText: true,
                  decoration: const InputDecoration(labelText: 'API token'),
                  onSubmitted: (_) {
                    if (!_busy && _settings != null) _connect();
                  },
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
