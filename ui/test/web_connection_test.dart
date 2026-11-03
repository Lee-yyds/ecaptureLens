@TestOn('browser')
library;

import 'dart:js_interop';

import 'package:ecapturelens/connection_web.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:web/web.dart' as web;

void main() {
  late String originalUrl;
  late JSAny? originalState;
  setUp(() {
    originalUrl = web.window.location.href;
    originalState = web.window.history.state;
    web.window.sessionStorage.removeItem('ecapturelens.token');
  });
  tearDown(() {
    web.window.history.replaceState(originalState, '', originalUrl);
    web.window.sessionStorage.removeItem('ecapturelens.token');
  });

  test('removing launch token preserves Flutter browser history state', () {
    web.window.history.replaceState(
      {
        'serialCount': 0,
        'state': {'route': 'bootstrap'},
      }.jsify(),
      '',
      '/#token=launch-token',
    );
    ConnectionSettings.load();
    expect(web.window.history.state.dartify(), {
      'serialCount': 0,
      'state': {'route': 'bootstrap'},
    });
    expect(web.window.location.hash, '');
  });

  test(
    'launch token is decoded, removed from URL and retained for refresh',
    () {
      web.window.sessionStorage.setItem('ecapturelens.token', 'stale-token');
      web.window.history.replaceState(
        null,
        '',
        '/?keep=yes#token=a%2Bb%2Fc%3D',
      );
      final settings = ConnectionSettings.load();
      expect(settings.token, 'a+b/c=');
      expect(settings.url, web.window.location.origin);
      expect(web.window.location.hash, '');
      expect(web.window.location.search, '?keep=yes');
      expect(web.window.sessionStorage.getItem('ecapturelens.token'), 'a+b/c=');
      expect(ConnectionSettings.load().token, 'a+b/c=');
    },
  );

  test('Go launch URL query escaping round trips punctuation and spaces', () {
    web.window.history.replaceState(
      null,
      '',
      '/#token=explicit+%2B%2F%23%3Ftoken',
    );
    expect(ConnectionSettings.load().token, 'explicit +/#?token');
    expect(web.window.location.hash, '');
  });

  test('manual token is tab scoped and disconnect clears it', () {
    web.window.history.replaceState(null, '', '/');
    final settings = ConnectionSettings.load();
    expect(settings.token, '');
    settings.save('manual-token');
    expect(ConnectionSettings.load().token, 'manual-token');
    settings.save('');
    expect(web.window.sessionStorage.getItem('ecapturelens.token'), isNull);
    expect(ConnectionSettings.load().token, '');
  });
}
