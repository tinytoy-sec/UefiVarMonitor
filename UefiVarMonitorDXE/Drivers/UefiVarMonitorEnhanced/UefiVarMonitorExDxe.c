#include "UefiVarMonitorExDxe.h"
#include <Uefi.h>
#include <Guid/EventGroup.h>
#include <Library/BaseLib.h>
#include <Library/BaseMemoryLib.h>
#include <Library/DebugLib.h>
#include <Library/MemoryAllocationLib.h>
#include <Library/PrintLib.h>
#include <Library/SynchronizationLib.h>
#include <Library/UefiBootServicesTableLib.h>
#include <Library/UefiLib.h>
#include <Library/UefiRuntimeServicesTableLib.h>
#include <library/UefiRuntimeLib.h>

//
// 256KB should be enough for every one, eh?
//
#define RUNTIME_BUFFER_SIZE_IN_PAGES    ((UINTN)64)
#define RUNTIME_BUFFER_SIZE_IN_BYTES    (RUNTIME_BUFFER_SIZE_IN_PAGES * EFI_PAGE_SIZE)

static EFI_EVENT g_SetVaMapEvent;
static EFI_GET_VARIABLE g_GetVariable;
static EFI_SET_VARIABLE g_SetVariable;

//
// Log buffer related.
//
static SPIN_LOCK g_LogBufferSpinLock;
static UINT8* g_LogBuffer;
static UINTN g_CurrentPoision;

//
// Callbacks.
//
static SPIN_LOCK g_VariableCallbacksLock;
static VARIABLE_CALLBACK g_VariableCallbacks[8];


#if defined(_MSC_VER)
//
// MSVC compiler intrinsics for CR8 access.
//
UINTN __readcr8(VOID);
VOID __writecr8(UINTN Data);

#elif defined(__GNUC__)
//
// Inline assemblies for CR8 access.
//
static
__inline__
__attribute__((always_inline))
UINTN
__readcr8 (
    VOID
    )
{
    UINTN value;

    __asm__ __volatile__ (
        "mov %%cr8, %[value]"
        : [value] "=a" (value)
    );
    return value;
}

static
__inline__
__attribute__((always_inline))
VOID
__writecr8 (
    UINTN Data
    )
{
    __asm__ __volatile__ (
        "mov %%eax, %%cr8"
        :
        : "a" (Data)
    );
}
#endif

/**
 * @brief Disables low-priority interrupts and acquires the spin lock,
 */
static
VOID
AcquireSpinLockForNt (
    IN OUT SPIN_LOCK* SpinLock,
    OUT UINTN* OldInterruptState
    )
{
    static const UINTN DISPATCH_LEVEL = 2;

    *OldInterruptState = __readcr8();
    ASSERT(*OldInterruptState <= DISPATCH_LEVEL);

    if (*OldInterruptState < DISPATCH_LEVEL) {
        __writecr8(DISPATCH_LEVEL);
    }
    
    AcquireSpinLock(SpinLock);
}

VOID
ReleaseSpinLockForNt (
    IN OUT SPIN_LOCK* SpinLock,
    IN UINTN OldInterruptState
    )
{
    ReleaseSpinLock(SpinLock);
    __writecr8((UINT8)OldInterruptState);
}
ReleaseSpinLockForNt (
    IN OUT SPIN_LOCK* SpinLock,
    IN UINTN NewInterruptState
    )
{
    ReleaseSpinLock(SpinLock);
    __writecr8(NewInterruptState);
}

/**
 * @brief Adds the new log entry to the global log buffer.
 */
static
VOID
AddLogEntryVariable(
    VARIABLE_CALLBACK_TYPE CallbackType,
    CONST CHAR16* VariableName,
    CONST EFI_GUID* VendorGuid,
    UINT32 Attributes,
    UINTN DataSize,
    CONST VOID* Data OPTIONAL,
    EFI_STATUS Status
    )
{
    UINTN interruptState;
    UINTN entrySize;
    VARIABLE_LOG_ENTRY* entry;

    AcquireSpinLockForNt(&g_LogBufferSpinLock, &interruptState);

    entrySize = sizeof(*entry) + DataSize;
    if ((g_CurrentPoision + entrySize) <= RUNTIME_BUFFER_SIZE_IN_BYTES)
    {
        //
        // Copy parameters to the log buffer.
        //
        entry = (VARIABLE_LOG_ENTRY*)&g_LogBuffer[g_CurrentPoision];
        StrnCpyS(entry->VariableName,
                 ARRAY_SIZE(entry->VariableName),
                 VariableName,
                 ARRAY_SIZE(entry->VariableName) - 1);
        entry->VendorGuid = *VendorGuid;
        entry->CallbackType = CallbackType;
        entry->Attributes = Attributes;
        entry->Status = Status;
        AsciiSPrint(entry->StatusMessage, sizeof(entry->StatusMessage), "%r", Status);
        entry->DataSize = DataSize;
        CopyMem(entry->Data, Data, DataSize);

        //
        // Log entries must start at 16 byte alignment.
        //
        g_CurrentPoision += ALIGN_VALUE(entrySize, 0x10);
        ASSERT((g_CurrentPoision % 0x10) == 0);
    }

    ReleaseSpinLockForNt(&g_LogBufferSpinLock, interruptState);

    DebugPrint(DEBUG_VERBOSE,
               "%c: %g Size=%08x %s: %r\n",
               (CallbackType == VariableCallbackGet) ? L'G' : L'S',
               VendorGuid,
               DataSize,
               VariableName,
               Status);
}

/**
 * @brief Moves the contents of the log buffer to the provided buffer.
 */
static
EFI_STATUS
HandleDrainBufferCommand (
    OUT VOID* Buffer OPTIONAL,
    IN OUT UINTN* BufferSize
    )
{
    EFI_STATUS status;
    UINTN interruptState;

    ASSERT((Buffer != NULL) || (*BufferSize == 0));

    //
    // Return the log buffer size if the provided buffer size is smaller than that.
    //
    if (*BufferSize < RUNTIME_BUFFER_SIZE_IN_BYTES)
    {
        *BufferSize = RUNTIME_BUFFER_SIZE_IN_BYTES;
        status = EFI_BUFFER_TOO_SMALL;
        goto Exit;
    }

    //
    // Copy the contents of log buffer to provided buffer and update the
    // buffer size with the current size. Then, reset the log buffer and the
    // current size.
    //
    AcquireSpinLockForNt(&g_LogBufferSpinLock, &interruptState);

    CopyMem(Buffer, g_LogBuffer, g_CurrentPoision);
    *BufferSize = g_CurrentPoision;
    ZeroMem(g_LogBuffer, g_CurrentPoision);
    g_CurrentPoision = 0;

    ReleaseSpinLockForNt(&g_LogBufferSpinLock, interruptState);

    status = EFI_SUCCESS;

Exit:
    return status;
}

/**
 * @brief Registers the callbacks of Get/SetVariable.
 */
static
EFI_STATUS
HandleRegisterCallbacksCommand (
    OUT VOID* Buffer OPTIONAL,
    IN OUT UINTN* BufferSize
    )
{
    EFI_STATUS status;
    UINTN interruptState;
    VARIABLE_CALLBACK callback;

    if ((Buffer == NULL) ||
        (*BufferSize != sizeof(VARIABLE_CALLBACK*)))
    {
        status = EFI_INVALID_PARAMETER;
        goto Exit;
    }

    //
    // Return this error when no slot is available.
    //
    status = EFI_OUT_OF_RESOURCES;
    callback = *(VARIABLE_CALLBACK*)Buffer;

    AcquireSpinLockForNt(&g_VariableCallbacksLock, &interruptState);

    for (UINTN i = 0; i < ARRAY_SIZE(g_VariableCallbacks); i++)
    {
        //
        // Register the callback if an empty slot is found.
        //
        if (g_VariableCallbacks[i] == NULL)
        {
            g_VariableCallbacks[i] = callback;
            status = EFI_SUCCESS;
            break;
        }

        //
        // Return error if the same callback is already registered.
        //
        if (g_VariableCallbacks[i] == callback)
        {
            status = EFI_INVALID_PARAMETER;
            break;
        }
    }

    ReleaseSpinLockForNt(&g_VariableCallbacksLock, interruptState);

    status = EFI_SUCCESS;

Exit:
    return status;
}

/**
 * @brief Unregisters the callback of Get/SetVariable.
 */
static
EFI_STATUS
HandleUnregisterCallbacksCommand (
    OUT VOID* Buffer OPTIONAL,
    IN OUT UINTN* BufferSize
    )
{
    EFI_STATUS status;
    UINTN interruptState;
    VARIABLE_CALLBACK callback;

    if ((Buffer == NULL) ||
        (*BufferSize != sizeof(VARIABLE_CALLBACK*)))
    {
        status = EFI_INVALID_PARAMETER;
        goto Exit;
    }

    //
    // Return this error when no slot is available.
    //
    status = EFI_INVALID_PARAMETER;
    callback = *(VARIABLE_CALLBACK*)Buffer;

    AcquireSpinLockForNt(&g_VariableCallbacksLock, &interruptState);

    for (UINTN i = 0; i < ARRAY_SIZE(g_VariableCallbacks); i++)
    {
        //
        // Clear the callback if the same ones as specified are found.
        //
        if (g_VariableCallbacks[i] == callback)
        {
            g_VariableCallbacks[i] = NULL;
            status = EFI_SUCCESS;
            break;
        }
    }

    ReleaseSpinLockForNt(&g_VariableCallbacksLock, interruptState);


Exit:
    return status;
}

/**
 * @brief Handles the backdoor command.
 */
static
EFI_STATUS
HandleBackdoorRequest (
    IN CONST CHAR16* VariableName,
    IN OUT VOID* Data OPTIONAL,
    IN OUT UINTN* DataSize
    )
{
    EFI_STATUS status;

    if (StrCmp(VariableName, L"RegisterCallbacks") == 0)
    {
        status = HandleRegisterCallbacksCommand(Data, DataSize);
    }
    else if (StrCmp(VariableName, L"UnregisterCallbacks") == 0)
    {
        status = HandleUnregisterCallbacksCommand(Data, DataSize);
    }
    else if (StrCmp(VariableName, L"DrainBuffer") == 0)
    {
        status = HandleDrainBufferCommand(Data, DataSize);
    }
    else
    {
        status = EFI_INVALID_PARAMETER;
    }

    return status;
}

/**
 * @brief Invokes all registered callbacks.
 */
static
EFI_STATUS
InvokeCallbacks (
    IN OUT VARIABLE_CALLBACK_PARAMETERS* Parameters
    )
{
    UINTN interruptState;
    BOOLEAN blocked;

    blocked = FALSE;

    AcquireSpinLockForNt(&g_VariableCallbacksLock, &interruptState);

    for (UINTN i = 0; i < ARRAY_SIZE(g_VariableCallbacks); i++)
    {
        if (g_VariableCallbacks[i] == NULL)
        {
            continue;
        }

        //
        // Invoke a callback. The blocked status cannot be override if any of
        // callbacks returned TRUE.
        //
        blocked |= g_VariableCallbacks[i](Parameters);
    }

    ReleaseSpinLockForNt(&g_VariableCallbacksLock, interruptState);

    //
    // Return EFI_ACCESS_DENIED if any of callbacks returned TRUE.
    //
    return (blocked == FALSE) ? EFI_SUCCESS : EFI_ACCESS_DENIED;
}

/**
 * @brief Invokes all registered Get callbacks.
 */
static
EFI_STATUS
InvokeGetCallbacks (
    IN OPERATION_TYPE OperationType,
    IN OUT CHAR16** VariableName,
    IN OUT EFI_GUID** VendorGuid,
    IN OUT UINT32** Attributes OPTIONAL,
    IN OUT UINTN** DataSize,
    IN OUT VOID** Data OPTIONAL,
    IN CONST EFI_STATUS* ResultStatus OPTIONAL
    )
{
    BOOLEAN succeeded;
    CHAR8 statusMessage[32];
    CHAR8* statusMessagePtr;
    VARIABLE_CALLBACK_PARAMETERS parameters;

    //
    // Convert the resulted status to a human readable string if it is given.
    //
    if (ResultStatus != NULL)
    {
        AsciiSPrint(statusMessage, sizeof(statusMessage), "%r", *ResultStatus);
        statusMessagePtr = statusMessage;
        succeeded = (EFI_ERROR(*ResultStatus) == FALSE);
    }
    else
    {
        statusMessagePtr = NULL;
        succeeded = FALSE;
    }

    parameters.CallbackType = VariableCallbackGet;
    parameters.OperationType = OperationType;
    parameters.Parameters.Get.VariableName = VariableName;
    parameters.Parameters.Get.VendorGuid = VendorGuid;
    parameters.Parameters.Get.Attributes = Attributes;
    parameters.Parameters.Get.DataSize = DataSize;
    parameters.Parameters.Get.Data = Data;
    parameters.Parameters.Get.Succeeded = succeeded;
    parameters.Parameters.Get.StatusMessage = statusMessagePtr;

    return InvokeCallbacks(&parameters);
}

/**
 * @brief Invokes all registered Set callbacks.
 */
static
EFI_STATUS
ProcessSetCallbacks (
    IN OPERATION_TYPE OperationType,
    IN OUT CHAR16** VariableName,
    IN OUT EFI_GUID** VendorGuid,
    IN OUT UINT32* Attributes,
    IN OUT UINTN* DataSize,
    IN OUT VOID** Data,
    IN CONST EFI_STATUS* ResultStatus OPTIONAL
    )
{
    BOOLEAN succeeded;
    CHAR8 statusMessage[32];
    CHAR8* statusMessagePtr;
    VARIABLE_CALLBACK_PARAMETERS parameters;

    //
    // Convert the resulted status to a human readable string if it is given.
    //
    if (ResultStatus != NULL)
    {
        AsciiSPrint(statusMessage, sizeof(statusMessage), "%r", *ResultStatus);
        statusMessagePtr = statusMessage;
        succeeded = (EFI_ERROR(*ResultStatus) == FALSE);
    }
    else
    {
        statusMessagePtr = NULL;
        succeeded = FALSE;
    }

    parameters.CallbackType = VariableCallbackSet;
    parameters.OperationType = OperationType;
    parameters.Parameters.Set.VariableName = VariableName;
    parameters.Parameters.Set.VendorGuid = VendorGuid;
    parameters.Parameters.Set.Attributes = Attributes;
    parameters.Parameters.Set.DataSize = DataSize;
    parameters.Parameters.Set.Data = Data;
    parameters.Parameters.Set.Succeeded = succeeded;
    parameters.Parameters.Set.StatusMessage = statusMessagePtr;

    return InvokeCallbacks(&parameters);
}

/**
 * @brief Handles GetVariable runtime service calls.
 */
static
EFI_STATUS
EFIAPI
HandleGetVariable (
    IN CHAR16* VariableName,
    IN EFI_GUID* VendorGuid,
    OUT UINT32* Attributes OPTIONAL,
    IN OUT UINTN* DataSize,
    OUT VOID* Data OPTIONAL
    )
{
    EFI_STATUS status;
    UINTN effectiveDataSize;
    UINT32 effectiveAttributes;

    //
    // Only execute a backdoor command if the certain GUID is specified.
    //
    if (CompareGuid(VendorGuid, &g_BackdoorGuid) != FALSE)
    {
        status = HandleBackdoorRequest(VariableName, Data, DataSize);
        goto Exit;
    }

    //
    // Invoke Pre- Get callbacks. Callbacks can make the service call fail.
    //
    status = InvokeGetCallbacks(OperationPre,
                                &VariableName,
                                &VendorGuid,
                                &Attributes,
                                &DataSize,
                                &Data,
                                NULL);
    if (EFI_ERROR(status))
    {
        goto Exit;
    }

    //
    // Invoke the original GetVariable service, and log this service invocation.
    //
    status = g_GetVariable(VariableName, VendorGuid, Attributes, DataSize, Data);
    effectiveDataSize = EFI_ERROR(status) ? 0 : *DataSize;
    effectiveAttributes = (EFI_ERROR(status) || (Attributes == NULL)) ? 0 : *Attributes;
    AddLogEntryVariable(VariableCallbackGet,
                        VariableName,
                        VendorGuid,
                        effectiveAttributes,
                        effectiveDataSize,
                        Data,
                        status);

    //
    // Invoke Post- Get callbacks. Post callbacks cannot make the service fail.
    //
    InvokeGetCallbacks(OperationPost,
                       &VariableName,
                       &VendorGuid,
                       &Attributes,
                       &DataSize,
                       &Data,
                       &status);

Exit:
    return status;
}

/**
 * @brief Handles SetVariable runtime service calls.
 */
static
EFI_STATUS
EFIAPI
HandleSetVariable (
    IN CHAR16* VariableName,
    IN EFI_GUID* VendorGuid,
    IN UINT32 Attributes,
    IN UINTN DataSize,
    IN VOID* Data
    )
{
    EFI_STATUS status;

    //
    // Invoke Pre- Set callbacks. Callbacks can make the service call fail.
    //
    status = ProcessSetCallbacks(OperationPre,
                                 &VariableName,
                                 &VendorGuid,
                                 &Attributes,
                                 &DataSize,
                                 &Data,
                                 NULL);
    if (EFI_ERROR(status))
    {
        goto Exit;
    }

    //
    // Invoke the original SetVariable service, and log this service invocation.
    //
    status = g_SetVariable(VariableName, VendorGuid, Attributes, DataSize, Data);
    AddLogEntryVariable(VariableCallbackSet,
                        VariableName,
                        VendorGuid,
                        Attributes,
                        DataSize,
                        Data,
                        status);

    //
    // Invoke Post- Set callbacks. Post callbacks cannot make the service fail.
    //
    ProcessSetCallbacks(OperationPost,
                        &VariableName,
                        &VendorGuid,
                        &Attributes,
                        &DataSize,
                        &Data,
                        &status);

Exit:
    return status;
}

/**
 * @brief Converts global pointers from physical-mode ones to virtual-mode ones.
 */
static
VOID
EFIAPI
HandleSetVirtualAddressMap (
    IN EFI_EVENT Event,
    IN VOID* Context
    )
{
    EFI_STATUS status;
    VOID* currentAddress;

    currentAddress = (VOID*)g_GetVariable;
    status = gRT->ConvertPointer(0, (VOID**)&g_GetVariable);
    ASSERT_EFI_ERROR(status);
    DEBUG((DEBUG_ERROR,
           "GetVariable relocated from %p to %p\n",
           currentAddress,
           g_GetVariable));

    currentAddress = (VOID*)g_SetVariable;
    status = gRT->ConvertPointer(0, (VOID**)&g_SetVariable);
    ASSERT_EFI_ERROR(status);
    DEBUG((DEBUG_ERROR,
           "SetVariable relocated from %p to %p\n",
           currentAddress,
           g_SetVariable));

    currentAddress = (VOID*)g_LogBuffer;
    status = gRT->ConvertPointer(0, (VOID**)&g_LogBuffer);
    ASSERT_EFI_ERROR(status);
    DEBUG((DEBUG_ERROR,
           "RuntimeBuffer relocated from %p to %p\n",
           currentAddress,
           g_LogBuffer));
}

/**
 * @brief Exchanges a pointer in the EFI System Table.
 */
static
EFI_STATUS
ExchangePointerInServiceTable (
    IN OUT VOID** AddressToUpdate,
    IN VOID* NewPointer,
    OUT VOID** OriginalPointer OPTIONAL
    )
{
    EFI_STATUS status;
    EFI_TPL tpl;

    ASSERT(*AddressToUpdate != NewPointer);

    tpl = gBS->RaiseTPL(TPL_HIGH_LEVEL);

    //
    // Save the current value if needed.
    //
    if (OriginalPointer != NULL)
    {
        *OriginalPointer = *AddressToUpdate;
    }
    *AddressToUpdate = NewPointer;

    //
    // Update the CRC32 in the EFI System Table header.
    //
    gST->Hdr.CRC32 = 0;
    status = gBS->CalculateCrc32(&gST->Hdr, gST->Hdr.HeaderSize, &gST->Hdr.CRC32);
    ASSERT_EFI_ERROR(status);

    gBS->RestoreTPL(tpl);
    return status;
}

/**
 * @brief Cleans up changes made by this module and release resources.
 *
 * @details This function handles pertical clean up and is safe for multiple calls.
 */
static
VOID
Cleanup (
    VOID
    )
{
    EFI_STATUS status;

    ASSERT(EfiAtRuntime() == FALSE);

    if (gST->RuntimeServices->SetVariable == HandleSetVariable)
    {
        status = ExchangePointerInServiceTable(
                                    (VOID**)&gST->RuntimeServices->SetVariable,
                                    (VOID*)g_SetVariable,
                                    NULL);
        ASSERT_EFI_ERROR(status);
    }

    if (gST->RuntimeServices->GetVariable == HandleGetVariable)
    {
        status = ExchangePointerInServiceTable(
                                    (VOID**)&gST->RuntimeServices->GetVariable,
                                    (VOID*)g_GetVariable,
                                    NULL);
        ASSERT_EFI_ERROR(status);
    }

    if (g_SetVaMapEvent != NULL)
    {
        status = gBS->CloseEvent(&g_SetVaMapEvent);
        g_SetVaMapEvent = NULL;
        ASSERT_EFI_ERROR(status);
    }

    if (g_LogBuffer != NULL)
    {
        FreePages(g_LogBuffer, RUNTIME_BUFFER_SIZE_IN_PAGES);
        g_LogBuffer = NULL;
    }
}

/**
 * @brief The module entry point.
 */
EFI_STATUS
EFIAPI
UefiVarMonitorDxeInitialize (
    IN EFI_HANDLE ImageHandle,
    IN EFI_SYSTEM_TABLE* SystemTable
    )
{
    EFI_STATUS status;

    InitializeSpinLock(&g_LogBufferSpinLock);
    InitializeSpinLock(&g_VariableCallbacksLock);

    DEBUG((DEBUG_ERROR, "Driver being loaded\n"));

    //
    // Allocate the memory that is availabe for use even at the runtime phase.
    //
    g_LogBuffer = AllocateRuntimePages(RUNTIME_BUFFER_SIZE_IN_PAGES);
    if (g_LogBuffer == NULL)
    {
        status = EFI_OUT_OF_RESOURCES;
        DEBUG((DEBUG_ERROR, "AllocateRuntimePages failed\n"));
        goto Exit;
    }
    ZeroMem(g_LogBuffer, RUNTIME_BUFFER_SIZE_IN_PAGES * EFI_PAGE_SIZE);

    //
    // Register a notification for SetVirtualAddressMap call.
    //
    status = gBS->CreateEventEx(EVT_NOTIFY_SIGNAL,
                                TPL_CALLBACK,
                                HandleSetVirtualAddressMap,
                                NULL,
                                &gEfiEventVirtualAddressChangeGuid,
                                &g_SetVaMapEvent);
    if (EFI_ERROR(status))
    {
        DEBUG((DEBUG_ERROR, "CreateEventEx failed : %r\n", status));
        goto Exit;
    }

    //
    // Install hooks. At this point, everything that is used in the hook handlers
    // must be initialized.
    //
    status = ExchangePointerInServiceTable((VOID**)&gST->RuntimeServices->GetVariable,
                                           (VOID*)HandleGetVariable,
                                           (VOID**)&g_GetVariable);
    if (EFI_ERROR(status))
    {
        DEBUG((DEBUG_ERROR, "ExchangeTablePointer(GetVariable) failed : %r\n", status));
        goto Exit;
    }
    status = ExchangePointerInServiceTable((VOID**)&gST->RuntimeServices->SetVariable,
                                           (VOID*)HandleSetVariable,
                                           (VOID**)&g_SetVariable);
    if (EFI_ERROR(status))
    {
        DEBUG((DEBUG_ERROR, "ExchangeTablePointer(SetVariable) failed : %r\n", status));
        goto Exit;
    }

Exit:
    if (EFI_ERROR(status))
    {
        Cleanup();
    }
    return status;
}

/**
 * @brief Handles unload request of this module.
 */
EFI_STATUS
EFIAPI
UefiVarMonitorDxeUnload (
    IN EFI_HANDLE ImageHandle
    )
{
    Cleanup();
    return EFI_SUCCESS;
}
