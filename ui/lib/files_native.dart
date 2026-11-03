import 'dart:io';
import 'dart:typed_data';

import 'package:file_picker/file_picker.dart';

Future<void> saveBytes(String name, List<int> bytes) async {
  final data = Uint8List.fromList(bytes);
  final path = await FilePicker.platform.saveFile(
    dialogTitle: 'Export $name',
    fileName: name,
    bytes: data,
  );
  // Android's document provider writes bytes; desktop returns a chosen path.
  if (path != null && !Platform.isAndroid && !Platform.isIOS) {
    await File(path).writeAsBytes(data, flush: true);
  }
}
