import React from "react";
import { Modal, Button } from "@heroui/react";
import { Trash2 } from "lucide-react";

export function DeleteConfirmModal({
  isOpen,
  title,
  targetName,
  description,
  onCancel,
  onConfirm,
}: {
  isOpen: boolean;
  title: string;
  targetName: string;
  description: string;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  return (
    <Modal>
      <Modal.Backdrop
        isOpen={isOpen}
        onOpenChange={(open) => {
          if (!open) onCancel();
        }}
        variant="blur"
      >
        <Modal.Container size="sm">
          <Modal.Dialog className="confirm-dialog" role="alertdialog">
            {(dialog: { close: () => void }) => (
              <>
                <Modal.CloseTrigger />
                <Modal.Header>
                  <Modal.Heading>{title}</Modal.Heading>
                </Modal.Header>
                <Modal.Body className="confirm-body">
                  <p className="confirm-message">
                    确认删除 <span className="code confirm-target">{targetName}</span> 吗？
                  </p>
                  <p className="muted confirm-warning">此操作不能直接撤销。{description}</p>
                </Modal.Body>
                <Modal.Footer>
                  <Button variant="tertiary" onPress={dialog.close}>
                    取消
                  </Button>
                  <Button
                    variant="danger"
                    onPress={() => {
                      onConfirm();
                      dialog.close();
                    }}
                  >
                    <Trash2 size={16} />
                    确认删除
                  </Button>
                </Modal.Footer>
              </>
            )}
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  );
}
