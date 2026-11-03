import 'package:flutter/material.dart';

import 'api.dart';
import 'connection_native.dart'
    if (dart.library.js_interop) 'connection_web.dart';
import 'workspace.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(const LensApp());
}

class LensApp extends StatelessWidget {
  const LensApp({super.key, this.client});
  final LensApi? client;

  @override
  Widget build(BuildContext context) {
    final light = ColorScheme.fromSeed(
      seedColor: const Color(0xff176b4b),
      surface: const Color(0xfffcfcfc),
    );
    final dark = ColorScheme.fromSeed(
      seedColor: const Color(0xff66dba7),
      brightness: Brightness.dark,
    );
    ThemeData theme(ColorScheme scheme) => ThemeData(
      colorScheme: scheme,
      useMaterial3: true,
      scaffoldBackgroundColor: scheme.surface,
      dividerColor: scheme.outlineVariant,
      appBarTheme: AppBarTheme(backgroundColor: scheme.surface),
      inputDecorationTheme: const InputDecorationTheme(
        border: OutlineInputBorder(),
        isDense: true,
      ),
    );
    return MaterialApp(
      title: 'eCaptureLens',
      debugShowCheckedModeBanner: false,
      theme: theme(light),
      darkTheme: theme(dark),
      home: client == null ? const ConnectionScreen() : Workspace(api: client!),
    );
  }
}
