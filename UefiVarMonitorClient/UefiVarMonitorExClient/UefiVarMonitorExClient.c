#include <UefiVarMonitorExDxe.h>
#include <ntstrsafe.h>
#include <intrin.h>
#include <ntddk.h>

/**
 * @brief Handles GetVariable and SetVariable runtime service calls.
 */
static
BOOLEAN
EFIAPI
HandleGetOrSetVariable (
    _Inout_ VARIABLE_CALLBACK_PARAMETERS* Parameters
    )
{
    NTSTATUS status;
    CHAR guidStr[RTL_GUID_STRING_SIZE - 2 + 1];  // -2 for {}, +1 for NULL
    CONST GUID* guid;
    SIZE_T dataSize;
    CONST WCHAR* variableName;
    CONST CHAR* message;
    LARGE_INTEGER timeStamp;

    //
    // This callback is always called at DISPATCH_LEVEL (to prevent recursive call).
    //
    NT_ASSERT(KeGetCurrentIrql() == DISPATCH_LEVEL);

    //
    // This sample does not do anything with the pre-callback.
    //
    if (Parameters->OperationType == OperationPre)
    {
        goto Exit;
    }

    //
    // Gather parameters and results to print them out.
    //
    if (Parameters->CallbackType == VariableCallbackGet)
    {
        guid = *Parameters->Parameters.Get.VendorGuid;
        dataSize = **Parameters->Parameters.Get.DataSize;
        variableName = *Parameters->Parameters.Get.VariableName;
        message = Parameters->Parameters.Get.StatusMessage;
    }
    else
    {
        guid = *Parameters->Parameters.Set.VendorGuid;
        dataSize = *Parameters->Parameters.Set.DataSize;
        variableName = *Parameters->Parameters.Set.VariableName;
        message = Parameters->Parameters.Set.StatusMessage;
    }

    status = RtlStringCchPrintfA(
        guidStr,
        RTL_NUMBER_OF(guidStr),
        "%08lX-%04hX-%04hX-%02hhX%02hhX-%02hhX%02hhX%02hhX%02hhX%02hhX%02hhX",
        guid->Data1,
        guid->Data2,
        guid->Data3,
        guid->Data4[0],
        guid->Data4[1],
        guid->Data4[2],
        guid->Data4[3],
        guid->Data4[4],
        guid->Data4[5],
        guid->Data4[6],
        guid->Data4[7]);
    NT_VERIFY(NT_SUCCESS(status));

    DbgPrintEx(DPFLTR_IHVDRIVER_ID,
               DPFLTR_ERROR_LEVEL,
               "%c: %s Size=%08X %S: %s\n",
               (Parameters->CallbackType == VariableCallbackGet) ? 'G' : 'S',
               guidStr,
               dataSize,
               variableName,
               message);

    // Get the current system time
    KeQuerySystemTime(&timeStamp);
    DbgPrintEx(DPFLTR_IHVDRIVER_ID,
               DPFLTR_ERROR_LEVEL,
               "Timestamp: %lld\n", timeStamp.QuadPart);

    // Log the variable access with timestamp
    DbgPrintEx(DPFLTR_IHVDRIVER_ID,
               DPFLTR_ERROR_LEVEL,
               "Access Time: %lld\n", timeStamp.QuadPart);

Exit:
    //
    // This callback always allows the original function to be called (returns FALSE) .
    //
    return FALSE;
}

/**
 * @brief Prints out log entries by parsing log buffer.
 */
static
VOID
ProcessBuffer (
    _In_ CONST UINT8* Buffer,
    _In_ ULONG EndOffset
    )
{
    PAGED_CODE();

    for (ULONG offset = 0; offset < EndOffset; )
    {
        NTSTATUS status;
        CONST VARIABLE_LOG_ENTRY* entry;
        CHAR guidStr[RTL_GUID_STRING_SIZE - 2 + 1];  // -2 for {}, +1 for NULL

        entry = (CONST VARIABLE_LOG_ENTRY*)&Buffer[offset];

        status = RtlStringCchPrintfA(
            guidStr,
            RTL_NUMBER_OF(guidStr),
            "%08lX-%04hX-%04hX-%02hhX%02hhX-%02hhX%02hhX%02hhX%02hhX%02hhX%02hhX",
            entry->VendorGuid.Data1,
            entry->VendorGuid.Data2,
            entry->VendorGuid.Data3,
            entry->VendorGuid.Data4[0],
            entry->VendorGuid.Data4[1],
            entry->VendorGuid.Data4[2],
            entry->VendorGuid.Data4[3],
            entry->VendorGuid.Data4[4],
            entry->VendorGuid.Data4[5],
            entry->VendorGuid.Data4[6],
            entry->VendorGuid.Data4[7]);
        NT_VERIFY(NT_SUCCESS(status));

        DbgPrintEx(DPFLTR_IHVDRIVER_ID,
                   DPFLTR_ERROR_LEVEL,
                   "%c: %s Size=%08X %S: %s\n",
                   (entry->CallbackType == VariableCallbackGet) ? 'G' : 'S',
                   guidStr,
                   entry->DataSize,
                   entry->VariableName,
                   entry->StatusMessage);

        offset += ALIGN_UP_BY(sizeof(*entry) + entry->DataSize, 0x10);
    }
}

/**
 * @brief Unloading entry point. Unregisters the registered callback.
 */
static
VOID
DriverUnload (
    _In_ PDRIVER_OBJECT DriverObject
    )
{
    NTSTATUS status;
    VOID* data;
    ULONG size;
    UNICODE_STRING unregisterCallbacks = RTL_CONSTANT_STRING(L"UnregisterCallbacks");

    UNREFERENCED_PARAMETER(DriverObject);

    PAGED_CODE();

    //
    // Unregister the callback.
    //
    data = (VOID*)&HandleGetOrSetVariable;
    size = sizeof(data);
    status = ExGetFirmwareEnvironmentVariable(&unregisterCallbacks,
                                              (GUID*)&g_BackdoorGuid,
                                              &data,
                                              &size,
                                              NULL);
    NT_ASSERT(NT_SUCCESS(status));
}

/**
 * @brief The module entry point. Prints out all buffered logs and registers a callback.
 */
NTSTATUS
DriverEntry (
    _In_ PDRIVER_OBJECT DriverObject,
    _In_ PUNICODE_STRING RegistryPath
    )
{
    NTSTATUS status;
    ULONG size;
    VOID* data;
    UINT8* buffer;
    UNICODE_STRING drainBuffer = RTL_CONSTANT_STRING(L"DrainBuffer");
    UNICODE_STRING registerCallbacks = RTL_CONSTANT_STRING(L"RegisterCallbacks");

    UNREFERENCED_PARAMETER(RegistryPath);

    buffer = NULL;

    DriverObject->DriverUnload = DriverUnload;

    //
    // Get a size of buffer required to drain contents of saved logs.
    //
    size = 0;
    status = ExGetFirmwareEnvironmentVariable(&drainBuffer,
                                              (GUID*)&g_BackdoorGuid,
                                              NULL,
                                              &size,
                                              NULL);
    if (status != STATUS_BUFFER_TOO_SMALL)
    {
        DbgPrintEx(DPFLTR_IHVDRIVER_ID,
                   DPFLTR_ERROR_LEVEL,
                   "ExGetFirmwareEnvironmentVariable returned unexpected value: %08x\n",
                   status);
        status = STATUS_UNSUCCESSFUL;
        goto Exit;
    }

    //
    // Allocate the buffer, get the contents, and parse it if there is any log.
    //
    if (size != 0)
    {
        buffer = ExAllocatePoolWithTag(PagedPool, size, 'CMVU');
        if (buffer == NULL)
        {
            DbgPrintEx(DPFLTR_IHVDRIVER_ID,
                       DPFLTR_ERROR_LEVEL,
                       "ExAllocatePoolWithTag failed : %08x\n", size);
            status = STATUS_INSUFFICIENT_RESOURCES;
            goto Exit;
        }

        status = ExGetFirmwareEnvironmentVariable(&drainBuffer,
                                                  (GUID*)&g_BackdoorGuid,
                                                  buffer,
                                                  &size,
                                                  NULL);
        if (!NT_SUCCESS(status))
        {
            DbgPrintEx(DPFLTR_IHVDRIVER_ID,
                       DPFLTR_ERROR_LEVEL,
                       "ExGetFirmwareEnvironmentVariable(DrainBuffer) failed : %08x\n",
                       status);
            goto Exit;
        }

        ProcessBuffer(buffer, size);
    }

    //
    // Register the callback.
    //
    data = (VOID*)&HandleGetOrSetVariable;
    size = sizeof(data);
    status = ExGetFirmwareEnvironmentVariable(&registerCallbacks,
                                              (GUID*)&g_BackdoorGuid,
                                              &data,
                                              &size,
                                              NULL);
    if (!NT_SUCCESS(status))
    {
        DbgPrintEx(DPFLTR_IHVDRIVER_ID,
                   DPFLTR_ERROR_LEVEL,
                   "ExGetFirmwareEnvironmentVariable(RegisterCallbacks) failed : %08x\n",
                   status);
        goto Exit;
    }

Exit:
    if (buffer != NULL)
    {
        ExFreePoolWithTag(buffer, 'CMVU');
    }
    return status;
}
